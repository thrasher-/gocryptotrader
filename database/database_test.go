package database

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// import sqlite3 driver
	_ "github.com/mattn/go-sqlite3"
)

func TestSetConfig(t *testing.T) {
	t.Parallel()
	inst := &Instance{}
	err := inst.SetConfig(&Config{Verbose: true})
	assert.NoError(t, err)

	err = inst.SetConfig(nil)
	assert.ErrorIs(t, err, ErrNilConfig)

	inst = nil
	err = inst.SetConfig(&Config{})
	assert.ErrorIs(t, err, ErrNilInstance)
}

func TestSetSQLiteConnection(t *testing.T) {
	t.Parallel()
	inst := &Instance{}
	err := inst.SetSQLiteConnection(nil)
	assert.ErrorIs(t, err, errNilSQL)

	err = inst.SetSQLiteConnection(&sql.DB{})
	assert.NoError(t, err)

	inst = nil
	err = inst.SetSQLiteConnection(nil)
	assert.ErrorIs(t, err, ErrNilInstance)
}

func TestSetPostgresConnection(t *testing.T) {
	// there is nothing actually requiring a postgres connection specifically
	// so this is testing the checks and the ability to set values
	// however, such settings would be bad for a sqlite connection irl
	t.Parallel()
	inst := &Instance{}
	databaseFullLocation := filepath.Join(DB.DataPath, "TestSetPostgresConnection")
	con, err := sql.Open("sqlite3", databaseFullLocation)
	require.NoError(t, err)
	err = inst.SetPostgresConnection(con)
	require.NoError(t, err)
	err = con.Close()
	require.NoError(t, err)
	err = os.Remove(databaseFullLocation)
	require.NoError(t, err)
}

func TestSetConnected(t *testing.T) {
	t.Parallel()
	inst := &Instance{}
	inst.SetConnected(true)
	if !inst.connected {
		t.Errorf("received %v, expected %v", false, true)
	}
	inst.SetConnected(false)
	if inst.connected {
		t.Errorf("received %v, expected %v", true, false)
	}
}

func TestCloseConnection(t *testing.T) {
	t.Parallel()
	inst := &Instance{}
	databaseFullLocation := filepath.Join(DB.DataPath, "TestCloseConnection")
	con, err := sql.Open("sqlite3", databaseFullLocation)
	require.NoError(t, err)
	err = inst.SetSQLiteConnection(con)
	require.NoError(t, err)
	err = inst.CloseConnection()
	require.NoError(t, err)
}

func TestIsConnected(t *testing.T) {
	t.Parallel()
	inst := &Instance{}

	inst.SetConnected(true)
	if !inst.IsConnected() {
		t.Errorf("received %v, expected %v", false, true)
	}
	inst.SetConnected(false)
	if inst.IsConnected() {
		t.Errorf("received %v, expected %v", true, false)
	}
}

func TestGetConfig(t *testing.T) {
	t.Parallel()
	inst := &Instance{}

	cfg := inst.GetConfig()
	if cfg != nil {
		t.Errorf("received %v, expected %v", cfg, nil)
	}

	err := inst.SetConfig(&Config{Enabled: true})
	require.NoError(t, err)

	cfg = inst.GetConfig()
	if cfg == nil {
		t.Errorf("received %v, expected %v", cfg, &Config{Enabled: true})
	}
}

func TestPing(t *testing.T) {
	t.Parallel()
	inst := &Instance{}
	databaseFullLocation := filepath.Join(DB.DataPath, "TestPing")
	con, err := sql.Open("sqlite3", databaseFullLocation)
	require.NoError(t, err)
	err = inst.SetSQLiteConnection(con)
	require.NoError(t, err)
	inst.SetConnected(true)
	err = inst.Ping()
	assert.NoError(t, err)
	inst.SQL = nil
	err = inst.Ping()
	assert.ErrorIs(t, err, errNilSQL)
	inst.SetConnected(false)
	err = inst.Ping()
	assert.ErrorIs(t, err, ErrDatabaseNotConnected)
	inst = nil
	err = inst.Ping()
	assert.ErrorIs(t, err, ErrNilInstance)
	err = con.Close()
	require.NoError(t, err)
	err = os.Remove(databaseFullLocation)
	require.NoError(t, err)
}

func TestGetSQL(t *testing.T) {
	t.Parallel()
	inst := &Instance{}
	_, err := inst.GetSQL()
	assert.ErrorIs(t, err, errNilSQL)

	databaseFullLocation := filepath.Join(DB.DataPath, "TestGetSQL")
	con, err := sql.Open("sqlite3", databaseFullLocation)
	require.NoError(t, err)
	err = inst.SetSQLiteConnection(con)
	require.NoError(t, err)
	_, err = inst.GetSQL()
	assert.NoError(t, err)

	inst = nil
	_, err = inst.GetSQL()
	assert.ErrorIs(t, err, ErrNilInstance)
}
