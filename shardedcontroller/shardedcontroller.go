package main

import (
	"crypto/sha512"
	"strings"
)

// rendezvous selects a shard from shards to handle item.
func rendezvous(item string, shards []string) string {
	maxWeight := uint64(0)
	maxShard := ""
	for _, shard := range shards {
		hash := sha512.Sum512_256(append([]byte(item), shard...))
		weight := uint64(0)
		for i := 0; i < 8; i++ {
			weight += uint64(hash[i]) << i * 8
		}

		if maxShard == "" {
			maxWeight = weight
			maxShard = shard
			continue
		}

		if weight > maxWeight {
			maxWeight = weight
			maxShard = shard
			continue
		}

		if weight == maxWeight && strings.Compare(shard, maxShard) > 0 {
			maxWeight = weight
			maxShard = shard
			continue
		}
	}

	return maxShard
}

func main() {
}
