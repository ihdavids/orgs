package orgs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/flosch/pongo2/v5"
)

func MathConstants() map[string]float64 {
	s := make(map[string]float64)
	s["e"] = 1.602176462e-19    // [C] Elementary charge
	s["c"] = 2.99792458e8       // Speed of light [m/s]
	s["h"] = 6.62606876e-34     // Planck's constant [J s]
	s["hbar"] = 1.054571596e-34 // Planck's constant [J s]
	s["GG"] = 6.673e-11         // Gravitational constant [N m^2/g^2]
	s["NA"] = 6.02214199e23     // Avogadro's constant [1/mol]
	s["me"] = 9.10938188e-31    // Electron rest mass [kg]
	s["mp"] = 1.67262158e-27    // Proton rest mass [kg]
	s["mn"] = 1.67492716e-27    // Neutron rest mass [kg]
	s["mu"] = 1.88353109e-28    // Muon rest mass [kg]
	s["amu"] = 1.66053873e-27   // Atomic mass unit [kg]
	s["Ryd"] = 1.09737315685e7  // Rydberg's constant [1/m]
	s["fsc"] = 7.297352533e-3   // Fine structure const []
	s["k"] = 1.3806503e-23      // Boltzmann's constant [J/K]
	s["R0"] = 8.314472e0        // Molar gas constant [J/K mol]
	s["V0"] = 2.2710981e-2      // Ideal gas volume [m^3/mol]
	s["sth"] = 6.6524e-29       // Thompson crosssection [m^2]
	s["sig"] = 5.6703e-8        // Stefan-Boltzman const [W/m^2 K^4]
	s["a"] = 7.5657e-15         // Radiation constant [J/m^3 K^4]
	// Math constants
	s["exp1"] = 2.7182818284590452354 // e (base of ln)
	// Length units
	s["m"] = 1.0e0 // Meter [m]
	// 1 lyr  = c * 365.2425 *24*60^2
	s["Ang"] = 1e-10 // Angstroem [m]
	s["mum"] = 1e-6  // Micrometer [m]
	// Just a few more commonly used english units - completeness is not attempted
	s["in"] = 2.54e-2        // Inch [m]
	s["ft"] = 3.048e-1       // Foot [m]
	s["yd"] = 9.144e-1       // Yard [m]
	s["mi"] = 1.609344e3     // Mile [m]
	s["nmi"] = 1.852e3       // Nautical Mile [m]
	s["pt"] = 3.527777778e-4 // Point (1/72 in) [m]
	// Area units
	s["hect"] = 1e4             // Hectar [m^2]
	s["acre"] = 4.04685642241e3 // Acre [m^2]
	s["ba"] = 1e-28             // Barn [m^2]
	// Time units
	s["s"] = 1.0e0      // Seconds [s]
	s["min"] = 60e0     // Minutes [s]
	s["hr"] = 3600e0    // Hours [s]
	s["d"] = 8.64e4     // Days [s]
	s["wk"] = 6.048e5   // Weeks [s]
	s["yr"] = 3.15576e7 // Years [s]
	s["Hz"] = 1.0e0     // Hertz [s]
	// Velocity Units
	s["kmh"] = 2.7777777778e-1 // Kilometers per  hour [m/s]
	s["mph"] = 4.4704e-1       // Miles per hour [m/s]
	s["knot"] = 5.144444444e-1 // Knot [m/s]
	// Mass units
	s["g"] = 1.0e-3           // Grams [kg]
	s["lb"] = 4.5359237e-1    // Pound [kg]
	s["oz"] = 2.8349523125e-2 // Ounce [kg]
	s["t"] = 1e3              // Metric ton [kg]
	s["ct"] = 2e-4            // Carat [kg]
	// Force units
	s["N"] = 1e0    // Newton (force) [kg m/s^2]
	s["dyn"] = 1e-5 // Dyne (force) [kg m/s^2]
	// Energy units
	s["J"] = 1e0              // Joule (energy) [J]
	s["erg"] = 1e-7           // Erg (energy) [J]
	s["cal"] = 4.1868e0       // Calories (energy) [J]
	s["eV"] = 1.602176462e-19 // Electron Volt (energy) [J]
	s["invcm"] = 1.986445e-23 // Energy in cm^-1 [J]
	s["Wh"] = 3.6e3           // Watt*Hour [J]
	s["hp"] = 7.457e2         // Horse power [J]
	s["Btu"] = 1.055056e10    // British Thermal Unit [J]
	// Power units
	s["W"] = 1e0 // Watt [J/s]
	// Pressure units
	s["Pa"] = 1e0              // Pascal (pressure) [N/m^2]
	s["bar"] = 1e5             // Bar (pressure) [N/m^2]
	s["atm"] = 1.01325e5       // Atmospheres (pressure) [N/m^2]
	s["torr"] = 1.333224e2     // Torr (pressure) [N/m^2]
	s["psi"] = 6.89475729317e3 // Pounds/in^2 [N/m^2]
	s["mHg"] = 1.333224e5      // Meter of Mercury [N/m^2]
	// Temperature units
	s["degK"] = 1.0e0           // Kelvin [K]
	s["degC"] = 1.0e0           // Celsius [K]
	s["degF"] = 0.55555555556e0 // Fahrenheit [K]
	// Light units
	s["cd"] = 1e0              // Candela [cd]
	s["sb"] = 1e4              // Stilb [cd/m^2]
	s["lm"] = 1e0              // Lumen [cd sr]
	s["lx"] = 1e0              // Lux [cd sr/m^2]
	s["ph"] = 1e4              // Phot [lx]
	s["lam"] = 3.18309886184e3 // Lambert [cd/m^2]
	// Radiation units
	s["Bq"] = 1.0e0   // Becquerel [1/s]
	s["Ci"] = 3.7e10  // Curie [1/s]
	s["Gy"] = 1.0e0   // Gray [J/kg]
	s["Sv"] = 1.0e0   // Sievert [J/kg]
	s["R"] = 2.58e-4  // Roentgen [C/kg]
	s["rd"] = 1.0e-2  // Rad (radiation) [J/kg]
	s["rem"] = 1.0e-2 // Rem [J/kg]
	// Amount of matter units"
	s["Mol"] = 1.0e0 // Mol (SI base unit) [mol]
	// Friction units"
	s["Poi"] = 1.0e-1 // Poise [kg/m s]
	s["St"] = 1.0e-4  // Stokes [m^2/s]
	// Electrical units"
	// Note: units refer to esu, not emu units....
	s["Amp"] = 1.0e0          // Ampere [A]
	s["C"] = 1.0e0            // Coulomb [C]
	s["Fdy"] = 9.6485341472e4 // Faraday [C]
	s["volt"] = 1.0e0         // Volt [W/A]
	s["ohm"] = 1.0e0          // Ohm [V/A]
	s["mho"] = 1.0e0          // Mho [A/V]
	s["siemens"] = 1.0e0      // Siemens [A/V]
	s["farad"] = 1.0e0        // Farad [C/V]
	s["henry"] = 1.0e0        // Henry [Wb/A]
	s["T"] = 1.0e0            // Tesla [Wb/m^2]
	s["gauss"] = 1.0e-4       // Gauss [Wb/m^2]
	s["Wb"] = 1.0e0           // Weber [V s]
	// Angular units
	s["rad"] = 1.0e0                 // Radian [rad]
	s["sr"] = 1.0e0                  // Steradian [sr]
	s["deg"] = 1.745329252e-2        // Degrees [rad]
	s["grad"] = 1.570796327e-2       // Grad [rad]
	s["arcmin"] = 2.908882087e-4     // Arcminutes [rad]
	s["arcsec"] = 4.848136812e-6     // Arcseconds [rad]
	s["deg2"] = 3.04617419786e-4     // Square Degrees [sr]
	s["arcmin2"] = 8.46159499406e-8  // Square Arcminutes [sr]
	s["arcsec2"] = 2.35044305389e-11 // Square Arcseconds [sr]
	// Astronomical Units
	s["lyr"] = 9.460536207e15 // Lightyear [m]
	// 1 pc       = AU / arcsec
	s["pc"] = 3.085677582e16   // Parsec [m]
	s["Lsun"] = 3.82e26        // Solar Luminosity [W]
	s["Msun"] = 1.989e30       // Solar Mass [kg]
	s["Mjup"] = 1.8986e27      // Jupiter mass [kg]
	s["Mea"] = 5.976e24        // Earth Mass [kg]
	s["Mmn"] = 7.35e22         // Moon mass [kg]
	s["Rsun"] = 6.96e8         // Solar radius [m]
	s["Rearth"] = 6.378e6      // Earth radius [m]
	s["Rmoon"] = 1.738e6       // Moon radius [m]
	s["Rjup"] = 7.1492e7       // Earth radius [m]
	s["AU"] = 1.49597870691e11 // Astronomical unit [m]
	s["Dmoon"] = 3.844e8       // Distance Earth-Moon [m]
	//s["Djup"]    = 7.78412d11        // Distance Sun-Jupiter [m]
	s["Jy"] = 1e-26     // Jansky [W / m^2 Hz]
	s["ga"] = 9.80665e0 // Earth acceleration [m/s^2]
	// Special Units
	// Planck units:  These definitions use h, not hbar
	s["lpl"] = 4.05083e-35 // Planck length (h) [m]
	s["mpl"] = 5.45621e-8  // Planck mass (h) [kg]
	s["tpl"] = 1.35121e-43 // Planck time (h) [s]
	// Planck units:  These definitions use hbar, not h
	s["lplb"] = 1.61605e-35 // Planck length (hbar) [m]
	s["mplb"] = 2.17671e-8  // Planck mass (hbar) [kg]
	s["tplb"] = 5.39056e-44 // Planck time (hbar) [s]
	return s
}

func resolveTemplate(name string, context map[string]string) string {
	tpl, _ := pongo2.FromFile(name)
	ctx := pongo2.Context{}
	for k, v := range context {
		ctx[k] = v
	}
	res, _ := tpl.Execute(ctx)
	return res
}

func RenderTemplate(name string, context map[string]string) string {
	tempName, _ := filepath.Abs(name)
	tempFolderName, _ := filepath.Abs(path.Join(Conf().TemplatePath, name))
	if _, err := os.Stat(name); err == nil {
		// Use name
	}
	if _, err := os.Stat(tempName); err == nil {
		// Try abs name of name
		name = tempName
	} else if _, err := os.Stat(tempFolderName); err == nil {
		// Try in the template folder for name
		name = tempFolderName
	}
	fmt.Printf("Trying to open path: %s\n", name)
	return resolveTemplate(name, context)
}
