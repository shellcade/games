package main

import "github.com/shellcade/kit/v2"

// config.go declares the pokies config surface (kit config key specs): the
// odds-variant PAR sheet, typed json with a default and a JSON Schema, so the
// arcade's admin tools render it as a rich form (weights as numeric fields,
// the paytable as editable rows) instead of a blind key/value prompt.

// defaultVariantJSON is the declared default for the odds-variant key — the
// JSON form of the compiled defaultDoc. A test pins that this document parses
// and compiles IDENTICAL to defaultVariant(), so the admin screen's "unset →
// default" can never drift from what the machine actually runs.
const defaultVariantJSON = `{
  "name": "Default",
  "weights": {"7": 1, "$": 2, "*": 3, "B": 6, "C": 13, "W": 1, "S": 2},
  "paytable": [
    {"faces": "7", "multiplier": 500},
    {"faces": "$", "multiplier": 150},
    {"faces": "*", "multiplier": 55},
    {"faces": "B", "multiplier": 10}
  ],
  "scatter": [
    {"count": 3, "spins": 8},
    {"count": 4, "spins": 15},
    {"count": 5, "spins": 25}
  ],
  "gamble": {"maxRungs": 5, "maxWin": 1000000}
}`

// oddsVariantSchema describes the PAR-sheet document within the admin rich-
// form supported subset: the regular symbols plus WILD (W) and SCATTER (S) as
// explicit weight properties (non-negative integers), the paytable as rows of
// {faces enum, multiplier ≥ 0}, an optional scatter free-spin trigger table, and
// optional gamble caps. Shape only — the semantic gates (≥1 positive weight,
// strip cap, retrigger convergence, total-RTP bounds) stay in compileVariant,
// the final word on every read.
const oddsVariantSchema = `{
  "type": "object",
  "required": ["name", "weights", "paytable"],
  "additionalProperties": false,
  "properties": {
    "name": {
      "type": "string",
      "minLength": 1,
      "description": "Variant label shown in admin summaries."
    },
    "weights": {
      "type": "object",
      "description": "Stops per symbol on the virtual reel strip. W is the wild, S the scatter.",
      "additionalProperties": false,
      "properties": {
        "7": {"type": "integer", "minimum": 0},
        "$": {"type": "integer", "minimum": 0},
        "*": {"type": "integer", "minimum": 0},
        "B": {"type": "integer", "minimum": 0},
        "C": {"type": "integer", "minimum": 0},
        "W": {"type": "integer", "minimum": 0},
        "S": {"type": "integer", "minimum": 0}
      }
    },
    "paytable": {
      "type": "array",
      "description": "Three-of-a-kind payouts, top-down (first match wins). Wilds substitute; an all-wild line pays the top multiplier.",
      "minItems": 1,
      "items": {
        "type": "object",
        "required": ["faces", "multiplier"],
        "additionalProperties": false,
        "properties": {
          "faces": {"type": "string", "enum": ["7", "$", "*", "B", "C"]},
          "multiplier": {"type": "integer", "minimum": 0}
        }
      }
    },
    "scatter": {
      "type": "array",
      "description": "Free-spin trigger table: count scatters anywhere in the 3x3 window award spins (highest matching count wins).",
      "items": {
        "type": "object",
        "required": ["count", "spins"],
        "additionalProperties": false,
        "properties": {
          "count": {"type": "integer", "minimum": 3},
          "spins": {"type": "integer", "minimum": 1}
        }
      }
    },
    "gamble": {
      "type": "object",
      "description": "Double-up ladder caps. Omitted blocks default to 5 rungs / 1,000,000 credits.",
      "additionalProperties": false,
      "properties": {
        "maxRungs": {"type": "integer", "minimum": 1},
        "maxWin": {"type": "integer", "minimum": 1}
      }
    }
  }
}`

// configSpecs is the declared config surface, returned from Meta().
func configSpecs() []kit.ConfigKeySpec {
	return []kit.ConfigKeySpec{{
		Key:         configKey, // "odds-variant"
		Title:       "Odds variant",
		Description: "PAR sheet: per-symbol reel weights (incl. wild W and scatter S), the three-of-a-kind paytable, the scatter free-spin table, and gamble caps. Applies to new rooms; running rooms refresh within 30s.",
		Type:        kit.ConfigJSON,
		Default:     defaultVariantJSON,
		Schema:      oddsVariantSchema,
	}}
}
