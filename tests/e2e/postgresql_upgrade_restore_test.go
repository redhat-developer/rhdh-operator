package e2e

import (
	"testing"

	"github.com/redhat-developer/rhdh-operator/tests/helper"
	"github.com/stretchr/testify/assert"
)

func TestPostgresqlSQLCommandUsesRequestedDatabase(t *testing.T) {
	cmd := postgresqlSQLCommand("test-namespace", "postgres-0", "backstage_plugin_catalog", "-c", "SELECT 1")

	assert.Equal(t, helper.GetPlatformTool(), cmd.Args[0])
	assert.Equal(t, []string{
		"-n", "test-namespace", "exec", "postgres-0", "--",
		"psql", "-X", "-U", "postgres", "-d", "backstage_plugin_catalog", "-c", "SELECT 1",
	}, cmd.Args[1:])
}

func TestPostgresqlSQLCommandWithStdinEnablesInteractiveExec(t *testing.T) {
	cmd := postgresqlSQLCommandWithStdin("test-namespace", "postgres-0", "postgres", "--quiet")

	assert.Equal(t, []string{
		"-n", "test-namespace", "exec", "-i", "postgres-0", "--",
		"psql", "-X", "-U", "postgres", "-d", "postgres", "--quiet",
	}, cmd.Args[1:])
}

func TestValidatePostgresqlRestoreErrors(t *testing.T) {
	tests := []struct {
		name    string
		stderr  string
		wantErr string
	}{
		{
			name:   "accepts bootstrap role conflict",
			stderr: "ERROR:  role \"postgres\" already exists\n",
		},
		{
			name:    "rejects an unexpected SQL error",
			stderr:  "ERROR:  role \"postgres\" already exists\nERROR:  relation \"example\" already exists\n",
			wantErr: "unexpected PostgreSQL restore errors",
		},
		{
			name:    "rejects a fatal connection error",
			stderr:  "ERROR:  role \"postgres\" already exists\nFATAL:  database \"backstage\" does not exist\n",
			wantErr: "unexpected PostgreSQL restore errors",
		},
		{
			name:    "rejects a psql connection error",
			stderr:  "ERROR:  role \"postgres\" already exists\npsql: error: connection to server was lost\n",
			wantErr: "unexpected PostgreSQL restore errors",
		},
		{
			name:    "rejects a restore without the expected bootstrap conflict",
			stderr:  "",
			wantErr: "expected one bootstrap role conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePostgresqlRestoreErrors(tt.stderr)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
