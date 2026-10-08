//go:build race

package rt

// raceOn: the race detector is on. sync.Pool drops items at random under
// it, so allocation budgets that count on a pool are not checked.
const raceOn = true
