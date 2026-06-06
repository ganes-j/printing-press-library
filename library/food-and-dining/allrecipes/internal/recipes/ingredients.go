package recipes

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ParsedIngredient is a structured form of one ingredient line.
type ParsedIngredient struct {
	Raw      string  `json:"raw"`
	Quantity float64 `json:"quantity,omitempty"`
	Unit     string  `json:"unit,omitempty"`
	Name     string  `json:"name"`
}

// fractionMap converts unicode and ASCII fractions to decimal.
var fractionMap = map[string]float64{
	"½": 0.5, "⅓": 1.0 / 3, "⅔": 2.0 / 3,
	"¼": 0.25, "¾": 0.75, "⅕": 0.2, "⅖": 0.4, "⅗": 0.6, "⅘": 0.8,
	"⅙": 1.0 / 6, "⅚": 5.0 / 6, "⅛": 0.125, "⅜": 0.375, "⅝": 0.625, "⅞": 0.875,
}

// units we recognize (lowercased). Order matters: longer first, so "tablespoons"
// matches before "tablespoon".
var unitsList = []string{
	"tablespoons", "tablespoon", "teaspoons", "teaspoon",
	"cups", "cup", "ounces", "ounce", "pounds", "pound",
	"grams", "gram", "kilograms", "kilogram",
	"milliliters", "milliliter", "liters", "liter",
	"pinch", "pinches", "dashes", "dash",
	"cloves", "clove", "sprigs", "sprig", "stalks", "stalk",
	"tbsp", "tsp", "tbs", "lb", "lbs", "oz", "ml", "g", "kg", "l",
}

// numberRe matches a decimal/integer/fraction-like prefix on an ingredient line.
var numberRe = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?(?:\s*\/\s*[0-9]+)?|[0-9]+\s+[0-9]+\s*\/\s*[0-9]+)`)

// ParseIngredient parses one ingredient line into ParsedIngredient. Best-effort:
// if no quantity or unit is detected, Name holds the full string and Quantity=0.
func ParseIngredient(line string) ParsedIngredient {
	pi := ParsedIngredient{Raw: line}
	s := strings.TrimSpace(line)
	if s == "" {
		return pi
	}

	// Replace unicode fractions with their decimal values, prefixed by space if
	// adjacent to digits.
	for ch, val := range fractionMap {
		if strings.Contains(s, ch) {
			s = strings.ReplaceAll(s, ch, " "+strconv.FormatFloat(val, 'f', -1, 64))
		}
	}
	s = strings.TrimSpace(s)

	// Try mixed-fraction "1 1/2"
	if m := regexp.MustCompile(`^(\d+)\s+(\d+)\s*/\s*(\d+)\s*`).FindStringSubmatch(s); m != nil {
		whole, _ := strconv.Atoi(m[1])
		num, _ := strconv.Atoi(m[2])
		den, _ := strconv.Atoi(m[3])
		if den != 0 {
			pi.Quantity = float64(whole) + float64(num)/float64(den)
			s = strings.TrimSpace(s[len(m[0]):])
		}
	}

	if pi.Quantity == 0 {
		if m := numberRe.FindStringSubmatch(s); m != nil {
			tok := strings.TrimSpace(m[1])
			if strings.Contains(tok, "/") {
				parts := strings.Split(tok, "/")
				if len(parts) == 2 {
					a, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
					b, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
					if b != 0 {
						pi.Quantity = a / b
					}
				}
			} else {
				v, _ := strconv.ParseFloat(tok, 64)
				pi.Quantity = v
			}
			s = strings.TrimSpace(s[len(m[0]):])
		}
	}

	// Extract unit.
	for _, u := range unitsList {
		lc := strings.ToLower(s)
		if strings.HasPrefix(lc, u+" ") || lc == u {
			pi.Unit = u
			s = strings.TrimSpace(s[len(u):])
			break
		}
	}

	pi.Name = strings.TrimSpace(s)
	if pi.Name == "" {
		pi.Name = pi.Raw
	}
	return pi
}

// ParseIngredients parses every ingredient line in a Recipe.
func ParseIngredients(lines []string) []ParsedIngredient {
	out := make([]ParsedIngredient, 0, len(lines))
	for _, l := range lines {
		out = append(out, ParseIngredient(l))
	}
	return out
}

// ScaleIngredients rescales every parsed ingredient's Quantity by factor.
// Returns new []ParsedIngredient with rescaled values; the Raw field is
// rewritten to a "qty unit name" string for friendly display.
func ScaleIngredients(items []ParsedIngredient, factor float64) []ParsedIngredient {
	if factor <= 0 || factor == 1 {
		return items
	}
	out := make([]ParsedIngredient, 0, len(items))
	for _, p := range items {
		scaled := p
		if p.Quantity > 0 {
			scaled.Quantity = roundQty(p.Quantity * factor)
			scaled.Raw = formatScaledIngredient(scaled)
		}
		out = append(out, scaled)
	}
	return out
}

// roundQty rounds a quantity to a kitchen-friendly precision: 2 decimals if
// fractional, integer otherwise.
func roundQty(q float64) float64 {
	if q == math.Floor(q) {
		return q
	}
	return math.Round(q*100) / 100
}

func formatScaledIngredient(p ParsedIngredient) string {
	parts := []string{}
	if p.Quantity > 0 {
		parts = append(parts, formatQty(p.Quantity))
	}
	if p.Unit != "" {
		parts = append(parts, p.Unit)
	}
	if p.Name != "" {
		parts = append(parts, p.Name)
	}
	return strings.Join(parts, " ")
}

func formatQty(q float64) string {
	if q == math.Floor(q) {
		return strconv.Itoa(int(q))
	}
	return strconv.FormatFloat(q, 'f', -1, 64)
}

// AggregateGrocery merges ingredient lists from multiple recipes. Items with
// matching (Unit, lower(Name)) are summed; items without unit are kept as
// counts. Items with mismatched units stay separate (grocery aggregation
// won't lie about converting cups to grams).
func AggregateGrocery(perRecipe [][]ParsedIngredient) []ParsedIngredient {
	type key struct{ unit, name string }
	totals := map[key]ParsedIngredient{}
	order := []key{}
	for _, items := range perRecipe {
		for _, p := range items {
			k := key{unit: strings.ToLower(p.Unit), name: strings.ToLower(p.Name)}
			cur, ok := totals[k]
			if !ok {
				totals[k] = p
				order = append(order, k)
				continue
			}
			cur.Quantity += p.Quantity
			cur.Raw = formatScaledIngredient(cur)
			totals[k] = cur
		}
	}
	out := make([]ParsedIngredient, 0, len(order))
	for _, k := range order {
		out = append(out, totals[k])
	}
	return out
}

// FormatTime turns a number of seconds into a kitchen-friendly "1h 30m" string.
func FormatTime(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}
