package registry

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Areas are stored as integer hundredths of a square metre ("centi", 52.30 м² = 5230):
// the registry gives areas with two decimals, integers keep sums exact.
// A vote weighs area × share; with shares like 1/3 the weight is a rational number,
// so weights are kept as numerator/denominator and summed with math/big.Rat.

// ErrInvalidArea is returned for an area that is not a positive number with at most two decimals.
var ErrInvalidArea = errors.New("invalid area")

// ErrInvalidShare is returned for a share outside (0, 1].
var ErrInvalidShare = errors.New("invalid share")

// ParseAreaCenti parses "52.3", "52,30" or "52" into hundredths of м².
func ParseAreaCenti(s string) (int64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	whole, frac, hasFrac := strings.Cut(s, ".")
	// Only digits: strconv.ParseInt also accepts a sign, and "52.+5" would become 52,05 м².
	if !digits(whole) || (hasFrac && (!digits(frac) || len(frac) > 2)) {
		return 0, fmt.Errorf("%w: %q", ErrInvalidArea, s)
	}
	for len(frac) < 2 {
		frac += "0"
	}

	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w < 0 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidArea, s)
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidArea, s)
	}

	c := w*100 + f
	if c <= 0 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidArea, s)
	}

	return c, nil
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

// Weight is the vote weight of an owner: area × share, in hundredths of м²,
// as an exact fraction Num/Den.
type Weight struct {
	Num int64
	Den int64
}

// NewWeight computes area × share. The share must be in (0, 1].
func NewWeight(areaCenti, shareNum, shareDen int64) (Weight, error) {
	if areaCenti <= 0 {
		return Weight{}, ErrInvalidArea
	}
	if shareNum <= 0 || shareDen <= 0 || shareNum > shareDen {
		return Weight{}, fmt.Errorf("%w: %d/%d", ErrInvalidShare, shareNum, shareDen)
	}

	return Weight{Num: areaCenti * shareNum, Den: shareDen}, nil
}

// M2 returns the weight in м² as an exact rational.
func (w Weight) M2() *big.Rat {
	return new(big.Rat).SetFrac64(w.Num, w.Den*100)
}

// CentiToM2 converts hundredths of м² to an exact rational in м².
func CentiToM2(c int64) *big.Rat {
	return new(big.Rat).SetFrac64(c, 100)
}

// FormatM2 formats an area for display: rounded to 0.01 м², decimal comma.
// Rounding is for display only; calculations use exact values.
func FormatM2(v *big.Rat) string {
	return strings.Replace(v.FloatString(2), ".", ",", 1)
}
