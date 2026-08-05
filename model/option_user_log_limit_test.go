package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUserLogQueryLimitDays(t *testing.T) {
	require.NoError(t, validateOptionValue("UserLogQueryLimitDays", "14"))
	require.NoError(t, validateOptionValue("UserLogQueryLimitDays", "3650"))
	assert.Error(t, validateOptionValue("UserLogQueryLimitDays", "0"))
	assert.Error(t, validateOptionValue("UserLogQueryLimitDays", "3651"))
	assert.Error(t, validateOptionValue("UserLogQueryLimitDays", "not-a-number"))
}
