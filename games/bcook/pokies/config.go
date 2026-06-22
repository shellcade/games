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
  "weights": {"7": 1, "$": 2, "*": 3, "B": 5, "C": 30, "W": 1, "S": 2},
  "paytable": [
    {"faces": "7", "pay3": 10, "pay4": 30, "pay5": 100},
    {"faces": "$", "pay3": 6, "pay4": 20, "pay5": 60},
    {"faces": "*", "pay3": 4, "pay4": 12, "pay5": 36},
    {"faces": "B", "pay3": 2, "pay4": 6, "pay5": 16}
  ],
  "scatter": [
    {"count": 3, "spins": 6},
    {"count": 4, "spins": 10},
    {"count": 5, "spins": 15}
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
      "description": "243-ways payouts: a left-aligned run of faces (wild substituting) pays pay3 / pay4 / pay5 x the ways for runs of 3 / 4 / 5 reels. First row per symbol wins.",
      "minItems": 1,
      "items": {
        "type": "object",
        "required": ["faces", "pay3", "pay4", "pay5"],
        "additionalProperties": false,
        "properties": {
          "faces": {"type": "string", "enum": ["7", "$", "*", "B", "C"]},
          "pay3": {"type": "integer", "minimum": 0},
          "pay4": {"type": "integer", "minimum": 0},
          "pay5": {"type": "integer", "minimum": 0}
        }
      }
    },
    "scatter": {
      "type": "array",
      "description": "Free-spin trigger table: count scatters anywhere in the 5x3 window award spins (highest matching count wins).",
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
		Description: "PAR sheet: per-symbol reel weights (incl. wild W and scatter S), the 243-ways paytable, the scatter free-spin table, and gamble caps. Applies to new rooms; running rooms refresh within 30s.",
		Type:        kit.ConfigJSON,
		Default:     defaultVariantJSON,
		Schema:      oddsVariantSchema,
	}}
}
