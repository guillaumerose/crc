package cmd

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"testing"

	"github.com/code-ready/crc/pkg/crc/machine/fakemachine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlainDelete(t *testing.T) {
	cacheDir, err := ioutil.TempDir("", "cache")
	require.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	out := new(bytes.Buffer)
	assert.NoError(t, runDelete(out, fakemachine.NewClient(), true, cacheDir, true, true, ""))
	assert.Equal(t, "Deleted the OpenShift cluster\n", out.String())

	_, err = os.Stat(cacheDir)
	assert.True(t, os.IsNotExist(err))
}

func TestNonForceDelete(t *testing.T) {
	cacheDir, err := ioutil.TempDir("", "cache")
	require.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	out := new(bytes.Buffer)
	assert.NoError(t, runDelete(out, fakemachine.NewClient(), true, cacheDir, true, false, ""))
	assert.Equal(t, "", out.String())

	_, err = os.Stat(cacheDir)
	assert.NoError(t, err)
}

func TestJSONDelete(t *testing.T) {
	cacheDir, err := ioutil.TempDir("", "cache")
	require.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	out := new(bytes.Buffer)
	assert.NoError(t, runDelete(out, fakemachine.NewClient(), true, cacheDir, false, true, jsonFormat))
	assert.JSONEq(t, `{"success": true}`, out.String())

	_, err = os.Stat(cacheDir)
	assert.True(t, os.IsNotExist(err))
}

func TestFailingPlainDelete(t *testing.T) {
	cacheDir, err := ioutil.TempDir("", "cache")
	require.NoError(t, err)
	defer os.RemoveAll(cacheDir)

	out := new(bytes.Buffer)
	client := fakemachine.NewClient()
	client.DontExist = true

	err = runDelete(out, client, true, cacheDir, true, true, "")

	var e1 *VirtualMachineNotFound
	assert.True(t, errors.As(err, &e1))
	var e2 *SerializableError
	assert.True(t, errors.As(err, &e2))
	assert.EqualError(t, err, "Machine does not exist. Use 'crc start' to create it")
}
