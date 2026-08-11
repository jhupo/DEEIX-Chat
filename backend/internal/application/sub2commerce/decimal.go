package sub2commerce

import (
	"encoding/json"
	"errors"
	"math/big"
	"strings"
)

const maxActualCostScale = 24

var errInvalidActualCost = errors.New("invalid Sub2 actual cost")

func exactActualCost(value json.Number) (string, error) {
	raw := strings.TrimSpace(value.String())
	if raw == "" {
		raw = "0"
	}
	return exactDecimal(raw)
}

func addActualCosts(left, right string) (string, error) {
	a, ok := new(big.Rat).SetString(left)
	if !ok {
		return "", errInvalidActualCost
	}
	b, ok := new(big.Rat).SetString(right)
	if !ok {
		return "", errInvalidActualCost
	}
	return formatFiniteDecimal(new(big.Rat).Add(a, b))
}

func exactDecimal(raw string) (string, error) {
	if len(raw) > 128 {
		return "", errInvalidActualCost
	}
	value, ok := new(big.Rat).SetString(raw)
	if !ok {
		return "", errInvalidActualCost
	}
	return formatFiniteDecimal(value)
}

func formatFiniteDecimal(value *big.Rat) (string, error) {
	denominator := new(big.Int).Set(value.Denom())
	two := big.NewInt(2)
	five := big.NewInt(5)
	remainder := new(big.Int)
	scale2, scale5 := 0, 0
	for {
		quotient := new(big.Int)
		quotient.QuoRem(denominator, two, remainder)
		if remainder.Sign() != 0 {
			break
		}
		denominator = quotient
		scale2++
	}
	for {
		quotient := new(big.Int)
		quotient.QuoRem(denominator, five, remainder)
		if remainder.Sign() != 0 {
			break
		}
		denominator = quotient
		scale5++
	}
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return "", errInvalidActualCost
	}
	scale := max(scale2, scale5)
	if scale > maxActualCostScale {
		return "", errInvalidActualCost
	}
	formatted := value.FloatString(scale)
	if strings.Contains(formatted, ".") {
		formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	}
	if formatted == "-0" || formatted == "" {
		return "0", nil
	}
	return formatted, nil
}
