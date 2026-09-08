package common

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyStorageViewsHaveIndependentCursors(t *testing.T) {
	storage, err := CreateBodyStorage([]byte("race-request"))
	require.NoError(t, err)
	defer storage.Close()

	first, err := NewBodyStorageView(storage)
	require.NoError(t, err)
	defer first.Close()
	second, err := NewBodyStorageView(storage)
	require.NoError(t, err)
	defer second.Close()

	firstPrefix := make([]byte, 4)
	_, err = io.ReadFull(first, firstPrefix)
	require.NoError(t, err)
	assert.Equal(t, "race", string(firstPrefix))

	secondBody, err := io.ReadAll(second)
	require.NoError(t, err)
	assert.Equal(t, "race-request", string(secondBody))

	firstRest, err := io.ReadAll(first)
	require.NoError(t, err)
	assert.Equal(t, "-request", string(firstRest))
}
