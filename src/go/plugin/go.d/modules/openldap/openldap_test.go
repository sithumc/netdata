// SPDX-License-Identifier: GPL-3.0-or-later

package openldap

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/go-ldap/ldap/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		assert.NotNil(t, data, name)
	}
}

func TestOpenLDAP_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &OpenLDAP{}, dataConfigJSON, dataConfigYAML)
}

func TestOpenLDAP_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails with default config": {
			wantFail: true,
			config:   New().Config,
		},
		"fails if URL not set": {
			wantFail: true,
			config: func() Config {
				conf := New().Config
				conf.URL = ""
				return conf
			}(),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			oldap := New()
			oldap.Config = test.config

			if test.wantFail {
				assert.Error(t, oldap.Init())
			} else {
				assert.NoError(t, oldap.Init())
			}
		})
	}
}

func TestOpenLDAP_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func() *OpenLDAP
	}{
		"not initialized": {
			prepare: func() *OpenLDAP {
				return New()
			},
		},
		"after check": {
			prepare: func() *OpenLDAP {
				oldap := New()
				oldap.newConn = func(Config) ldapConn { return prepareMockOk() }
				_ = oldap.Check()
				return oldap
			},
		},
		"after collect": {
			prepare: func() *OpenLDAP {
				oldap := New()
				oldap.newConn = func(Config) ldapConn { return prepareMockOk() }
				_ = oldap.Collect()
				return oldap
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			oldap := test.prepare()

			assert.NotPanics(t, oldap.Cleanup)
		})
	}
}

func TestOpenLDAP_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestOpenLDAP_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func() *mockOpenLDAPConn
		wantFail    bool
	}{
		"success case": {
			wantFail:    false,
			prepareMock: prepareMockOk,
		},
		"err on connect": {
			wantFail:    true,
			prepareMock: prepareMockErrOnConnect,
		},
		"err on search": {
			wantFail:    true,
			prepareMock: prepareMockErrOnSearch,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			oldap := New()
			mock := test.prepareMock()
			oldap.newConn = func(Config) ldapConn { return mock }

			if test.wantFail {
				assert.Error(t, oldap.Check())
			} else {
				assert.NoError(t, oldap.Check())
			}
		})
	}
}

func TestOpenLDAP_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock             func() *mockOpenLDAPConn
		wantMetrics             map[string]int64
		disconnectBeforeCleanup bool
		disconnectAfterCleanup  bool
	}{
		"success case": {
			prepareMock:             prepareMockOk,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantMetrics: map[string]int64{
				"bytes_sent":                   1,
				"completed_add_operations":     1,
				"completed_bind_operations":    1,
				"completed_compare_operations": 1,
				"completed_delete_operations":  1,
				"completed_modify_operations":  1,
				"completed_operations":         7,
				"completed_search_operations":  1,
				"completed_unbind_operations":  1,
				"current_connections":          1,
				"entries_sent":                 1,
				"initiated_add_operations":     1,
				"initiated_bind_operations":    1,
				"initiated_compare_operations": 1,
				"initiated_delete_operations":  1,
				"initiated_modify_operations":  1,
				"initiated_operations":         7,
				"initiated_search_operations":  1,
				"initiated_unbind_operations":  1,
				"read_waiters":                 1,
				"referrals_sent":               1,
				"total_connections":            1,
				"write_waiters":                1,
			},
		},
		"err on connect": {
			prepareMock:             prepareMockErrOnConnect,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  false,
		},
		"err on search": {
			prepareMock:             prepareMockErrOnSearch,
			disconnectBeforeCleanup: true,
			disconnectAfterCleanup:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			oldap := New()
			mock := test.prepareMock()
			oldap.newConn = func(Config) ldapConn { return mock }

			mx := oldap.Collect()

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, oldap.Charts(), mx)
			}

			assert.Equal(t, test.disconnectBeforeCleanup, mock.disconnectCalled, "disconnect before cleanup")
			oldap.Cleanup()
			assert.Equal(t, test.disconnectAfterCleanup, mock.disconnectCalled, "disconnect after cleanup")
		})
	}
}

func prepareMockOk() *mockOpenLDAPConn {
	return &mockOpenLDAPConn{
		dataSearchMonCounters: &ldap.SearchResult{
			Entries: []*ldap.Entry{
				{
					DN: "cn=Current,cn=Connections,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorCounter, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Total,cn=Connections,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorCounter, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Bytes,cn=Statistics,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorCounter, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Referrals,cn=Statistics,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorCounter, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Entries,cn=Statistics,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorCounter, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Write,cn=Waiters,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorCounter, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Read,cn=Waiters,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorCounter, Values: []string{"1"}},
					},
				},
			},
		},
		dataSearchMonOperations: &ldap.SearchResult{
			Entries: []*ldap.Entry{
				{
					DN: "cn=Bind,cn=Operations,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorOpInitiated, Values: []string{"1"}},
						{Name: attrMonitorOpCompleted, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Unbind,cn=Operations,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorOpInitiated, Values: []string{"1"}},
						{Name: attrMonitorOpCompleted, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Add,cn=Operations,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorOpInitiated, Values: []string{"1"}},
						{Name: attrMonitorOpCompleted, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Delete,cn=Operations,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorOpInitiated, Values: []string{"1"}},
						{Name: attrMonitorOpCompleted, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Modify,cn=Operations,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorOpInitiated, Values: []string{"1"}},
						{Name: attrMonitorOpCompleted, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Compare,cn=Operations,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorOpInitiated, Values: []string{"1"}},
						{Name: attrMonitorOpCompleted, Values: []string{"1"}},
					},
				},
				{
					DN: "cn=Search,cn=Operations,cn=Monitor",
					Attributes: []*ldap.EntryAttribute{
						{Name: attrMonitorOpInitiated, Values: []string{"1"}},
						{Name: attrMonitorOpCompleted, Values: []string{"1"}},
					},
				},
			},
		},
	}
}

func prepareMockErrOnConnect() *mockOpenLDAPConn {
	return &mockOpenLDAPConn{
		errOnConnect: true,
	}
}

func prepareMockErrOnSearch() *mockOpenLDAPConn {
	return &mockOpenLDAPConn{
		errOnSearch: true,
	}
}

type mockOpenLDAPConn struct {
	errOnConnect     bool
	disconnectCalled bool

	dataSearchMonCounters   *ldap.SearchResult
	dataSearchMonOperations *ldap.SearchResult
	errOnSearch             bool
}

func (m *mockOpenLDAPConn) connect() error {
	if m.errOnConnect {
		return errors.New("mock.connect() error")
	}
	return nil
}

func (m *mockOpenLDAPConn) disconnect() error {
	m.disconnectCalled = true
	return nil
}

func (m *mockOpenLDAPConn) search(req *ldap.SearchRequest) (*ldap.SearchResult, error) {
	if m.errOnSearch {
		return nil, errors.New("mock.search() error")
	}

	switch req.BaseDN {
	case "cn=Monitor":
		return m.dataSearchMonCounters, nil
	case "cn=Operations,cn=Monitor":
		return m.dataSearchMonOperations, nil
	default:
		return nil, fmt.Errorf("mock.search(): unknown BaseDSN: %s", req.BaseDN)
	}
}
