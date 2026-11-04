package main

// Ladder points and tiers (03 Game Design/Game Modes.md, D44).

// Tier floors, in LP.
var tierFloors = []int{0, 1000, 2500, 5000, 8000, 12000}

// TierNames are the academic ranks the world uses (04 Art/World and Theme.md).
var tierNames = []string{"Freshman", "Sophomore", "Junior", "Senior", "Graduate", "Master"}

// tierOf is the tier a rating sits in.
func tierOf(lp int) int {
	t := 0
	for i, floor := range tierFloors {
		if lp >= floor {
			t = i
		}
	}
	return t
}

// The rating change for a win, in LP.
const (
	lpBase = 30
	lpMin  = 10
	lpMax  = 50
	// LP of gap that moves the payout by one point.
	lpGapPerPoint = 40
)

func lpChange(winnerLP, loserLP int) int {
	change := lpBase + (loserLP-winnerLP)/lpGapPerPoint
	return min(max(change, lpMin), lpMax)
}

// applyLoss takes the points off, but never out of the tier the player was in.
func applyLoss(lp, change int) int {
	floor := tierFloors[tierOf(lp)]
	return max(lp-change, floor)
}
