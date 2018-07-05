package peerwatch

import (
    "sort"
    "fmt"
)

// podSet will hold set of ready pods' ip addresses
type podSet map[string]bool

func (podSet podSet) Keys() []string {
    keys := make([]string, len(podSet))
    i := 0
    for key := range podSet {
        keys[i] = key
        i++
    }
    sort.Strings(keys)
    return keys
}

func (podSet podSet) String() string {
    return fmt.Sprintf("%v", podSet.Keys())
}