package main

// The technobabble generator: control names are ADJECTIVE + NOUN, sampled
// without replacement across the whole ship so every order is unambiguous.
// All words are <= 11 runes so a label line always fits a widget's interior.
// Original wordlists — nothing here is borrowed from any other game.

var adjectives = []string{
	"GYROSCOPIC", "POLARIZED", "TACHYON", "FERROUS", "BEVELED",
	"OSMOTIC", "RECURSIVE", "DORSAL", "SUBSPACE", "IONIC",
	"QUANTUM", "AUXILIARY", "HELICAL", "MAGNETIC", "PHOTONIC",
	"CENTRIFUGAL", "PLASMIC", "NEWTONIAN", "VECTORED", "GRAVITIC",
	"INVERTED", "MODULATED", "TRIAXIAL", "LAMINAR", "PNEUMATIC",
	"CRYOGENIC", "HARMONIC", "OBLIQUE", "SIDEREAL", "KINETIC",
	"NEBULAR", "PAROTID", "VESTIBULAR", "TELESCOPIC", "ISOTOPIC",
	"FRACTAL", "ELLIPTIC", "CALIBRATED", "TURBULENT", "ANCILLARY",
	"PERFORATED", "RESONANT", "GALVANIC", "SPECTRAL", "TANGENTIAL",
	"HYDRAULIC", "ORBITAL", "CORRUGATED",
}

var nouns = []string{
	"PLURALIZER", "SLIPNOZZLE", "BELLOWS", "WHISK", "GRAVIMETER",
	"CROUTON", "DEFROSTER", "MANIFOLD", "HOLOSPINDLE", "NANOBUZZER",
	"FLANGE", "GIMBAL", "SPROCKET", "DAMPENER", "PHASELOOP",
	"THERMOCOUPLE", "WIDGETRON", "CAPACITOR", "FUNNEL", "REGULATOR",
	"OSCILLATOR", "PYLON", "SQUEEGEE", "INDUCTOR", "TUMBLER",
	"APERTURE", "SOLENOID", "CRUMPLER", "BAFFLE", "PERISCOPE",
	"DYNAMO", "IMPELLER", "GASKET", "SPIGOT", "RECTIFIER",
	"TROMBONE", "CANISTER", "HUMIDIFIER", "PENDULUM", "TUNING FORK",
	"ARMATURE", "COMPRESSOR", "LADLE", "TURBINE", "STABILIZER",
	"KAZOO", "WINCH", "DOORKNOB",
}

// Name suffixes stretch the pool (and force careful reading) at sector 5+.
var nameSuffixes = []string{" MK-II", " (AUX)"}

// sectorNames flavour the status strip; they cycle for very long runs.
var sectorNames = []string{
	"THE CRAB NEBULA",
	"THE GLASS SHOALS",
	"THE EMBER DRIFT",
	"THE HOWLING DEEP",
	"THE PALE EXPANSE",
	"THE SPIRAL FONT",
	"THE QUIET REACH",
	"THE BRASS VEIL",
	"THE SUNDER FIELD",
	"THE VIOLET RIFT",
	"THE IRON SARGASSO",
	"THE LAST SHALLOWS",
}

func sectorName(sector int) string {
	return sectorNames[(sector-1)%len(sectorNames)]
}
