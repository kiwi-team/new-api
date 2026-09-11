package model

import (
	"testing"

	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
)

func TestSelectChannelRuleGroupCandidates(t *testing.T) {
	reverse := func(ids []int) {
		for left, right := 0, len(ids)-1; left < right; left, right = left+1, right-1 {
			ids[left], ids[right] = ids[right], ids[left]
		}
	}

	tests := []struct {
		name        string
		mode        string
		count       int
		want        []int
		wantShuffle bool
	}{
		{name: "missing mode keeps legacy random polling", want: []int{4, 3, 2, 1}, wantShuffle: true},
		{name: "random polling uses every channel", mode: hostdto.ChannelRaceGroupModeRandom, want: []int{4, 3, 2, 1}, wantShuffle: true},
		{name: "ordered polling preserves configuration order", mode: hostdto.ChannelRaceGroupModeOrder, want: []int{1, 2, 3, 4}},
		{name: "random n limits candidates", mode: hostdto.ChannelRaceGroupModeRandomN, count: 2, want: []int{4, 3}, wantShuffle: true},
		{name: "random n defaults to one", mode: hostdto.ChannelRaceGroupModeRandomN, want: []int{4}, wantShuffle: true},
		{name: "random n above group size uses all", mode: hostdto.ChannelRaceGroupModeRandomN, count: 10, want: []int{4, 3, 2, 1}, wantShuffle: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuffled := false
			shuffle := func(ids []int) {
				shuffled = true
				reverse(ids)
			}

			got := selectChannelRuleGroupCandidates([]int{1, 2, 3, 4}, tt.mode, tt.count, shuffle)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantShuffle, shuffled)
		})
	}
}

func TestFlattenChannelRuleGroups(t *testing.T) {
	reverseGroups := func(groups [][]int) {
		for left, right := 0, len(groups)-1; left < right; left, right = left+1, right-1 {
			groups[left], groups[right] = groups[right], groups[left]
		}
	}
	reverseChannels := func(channelIds []int) {
		for left, right := 0, len(channelIds)-1; left < right; left, right = left+1, right-1 {
			channelIds[left], channelIds[right] = channelIds[right], channelIds[left]
		}
	}

	t.Run("ordered routing preserves group and candidate order", func(t *testing.T) {
		got := flattenChannelRuleGroups([][]int{{1, 2}, {3, 4}}, "order", true, reverseGroups, reverseChannels)

		assert.Equal(t, []int{1, 2, 3, 4}, got)
	})

	t.Run("legacy random routing keeps whole-list shuffle behavior", func(t *testing.T) {
		got := flattenChannelRuleGroups([][]int{{1, 2}, {3, 4}}, "random", false, reverseGroups, reverseChannels)

		assert.Equal(t, []int{4, 3, 2, 1}, got)
	})

	t.Run("random routing with group strategies randomizes groups without breaking their candidate order", func(t *testing.T) {
		got := flattenChannelRuleGroups([][]int{{1, 2}, {3, 4}}, "random", true, reverseGroups, reverseChannels)

		assert.Equal(t, []int{3, 4, 1, 2}, got)
	})
}
