package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldSpecialChannelsOverrideKeyRules(t *testing.T) {
	tests := []struct {
		name                 string
		specialChannelIds    []int
		keyChannelIds        []int
		keyRulesHighPriority bool
		want                 bool
	}{
		{
			name:              "special rules keep their existing default priority",
			specialChannelIds: []int{10},
			keyChannelIds:     []int{20},
			want:              true,
		},
		{
			name:                 "key rules override special rules when enabled",
			specialChannelIds:    []int{10},
			keyChannelIds:        []int{20},
			keyRulesHighPriority: true,
			want:                 false,
		},
		{
			name:                 "special rules remain available when key rules yield no channels",
			specialChannelIds:    []int{10},
			keyRulesHighPriority: true,
			want:                 true,
		},
		{
			name:          "no special rules means no override",
			keyChannelIds: []int{20},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldSpecialChannelsOverrideKeyRules(
				tt.specialChannelIds,
				tt.keyChannelIds,
				tt.keyRulesHighPriority,
			))
		})
	}
}
