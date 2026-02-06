package templates

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ekalinin/go-textwrap"
	"github.com/flosch/pongo2/v5"
	"github.com/muesli/reflow/wordwrap"
)

func MathConstants(prefix string) map[string]float64 {
	s := make(map[string]float64)
	s[prefix+"e"] = 1.602176462e-19    // [C] Elementary charge
	s[prefix+"c"] = 2.99792458e8       // Speed of light [m/s]
	s[prefix+"h"] = 6.62606876e-34     // Planck's constant [J s]
	s[prefix+"hbar"] = 1.054571596e-34 // Planck's constant [J s]
	s[prefix+"GG"] = 6.673e-11         // Gravitational constant [N m^2/g^2]
	s[prefix+"NA"] = 6.02214199e23     // Avogadro's constant [1/mol]
	s[prefix+"me"] = 9.10938188e-31    // Electron rest mass [kg]
	s[prefix+"mp"] = 1.67262158e-27    // Proton rest mass [kg]
	s[prefix+"mn"] = 1.67492716e-27    // Neutron rest mass [kg]
	s[prefix+"mu"] = 1.88353109e-28    // Muon rest mass [kg]
	s[prefix+"amu"] = 1.66053873e-27   // Atomic mass unit [kg]
	s[prefix+"Ryd"] = 1.09737315685e7  // Rydberg's constant [1/m]
	s[prefix+"fsc"] = 7.297352533e-3   // Fine structure const []
	s[prefix+"k"] = 1.3806503e-23      // Boltzmann's constant [J/K]
	s[prefix+"R0"] = 8.314472e0        // Molar gas constant [J/K mol]
	s[prefix+"V0"] = 2.2710981e-2      // Ideal gas volume [m^3/mol]
	s[prefix+"sth"] = 6.6524e-29       // Thompson crosssection [m^2]
	s[prefix+"sig"] = 5.6703e-8        // Stefan-Boltzman const [W/m^2 K^4]
	s[prefix+"a"] = 7.5657e-15         // Radiation constant [J/m^3 K^4]
	// Math constants
	s[prefix+"exp1"] = 2.7182818284590452354 // e (base of ln)
	// Length units
	s[prefix+"m"] = 1.0e0 // Meter [m]
	// 1 lyr  = c * 365.2425 *24*60^2
	s[prefix+"Ang"] = 1e-10 // Angstroem [m]
	s[prefix+"mum"] = 1e-6  // Micrometer [m]
	// Just a few more commonly used english units - completeness is not attempted
	s[prefix+"in"] = 2.54e-2        // Inch [m]
	s[prefix+"ft"] = 3.048e-1       // Foot [m]
	s[prefix+"yd"] = 9.144e-1       // Yard [m]
	s[prefix+"mi"] = 1.609344e3     // Mile [m]
	s[prefix+"nmi"] = 1.852e3       // Nautical Mile [m]
	s[prefix+"pt"] = 3.527777778e-4 // Point (1/72 in) [m]
	// Area units
	s[prefix+"hect"] = 1e4             // Hectar [m^2]
	s[prefix+"acre"] = 4.04685642241e3 // Acre [m^2]
	s[prefix+"ba"] = 1e-28             // Barn [m^2]
	// Time units
	s[prefix+"s"] = 1.0e0      // Seconds [s]
	s[prefix+"min"] = 60e0     // Minutes [s]
	s[prefix+"hr"] = 3600e0    // Hours [s]
	s[prefix+"d"] = 8.64e4     // Days [s]
	s[prefix+"wk"] = 6.048e5   // Weeks [s]
	s[prefix+"yr"] = 3.15576e7 // Years [s]
	s[prefix+"Hz"] = 1.0e0     // Hertz [s]
	// Velocity Units
	s[prefix+"kmh"] = 2.7777777778e-1 // Kilometers per  hour [m/s]
	s[prefix+"mph"] = 4.4704e-1       // Miles per hour [m/s]
	s[prefix+"knot"] = 5.144444444e-1 // Knot [m/s]
	// Mass units
	s[prefix+"g"] = 1.0e-3           // Grams [kg]
	s[prefix+"lb"] = 4.5359237e-1    // Pound [kg]
	s[prefix+"oz"] = 2.8349523125e-2 // Ounce [kg]
	s[prefix+"t"] = 1e3              // Metric ton [kg]
	s[prefix+"ct"] = 2e-4            // Carat [kg]
	// Force units
	s[prefix+"N"] = 1e0    // Newton (force) [kg m/s^2]
	s[prefix+"dyn"] = 1e-5 // Dyne (force) [kg m/s^2]
	// Energy units
	s[prefix+"J"] = 1e0              // Joule (energy) [J]
	s[prefix+"erg"] = 1e-7           // Erg (energy) [J]
	s[prefix+"cal"] = 4.1868e0       // Calories (energy) [J]
	s[prefix+"eV"] = 1.602176462e-19 // Electron Volt (energy) [J]
	s[prefix+"invcm"] = 1.986445e-23 // Energy in cm^-1 [J]
	s[prefix+"Wh"] = 3.6e3           // Watt*Hour [J]
	s[prefix+"hp"] = 7.457e2         // Horse power [J]
	s[prefix+"Btu"] = 1.055056e10    // British Thermal Unit [J]
	// Power units
	s[prefix+"W"] = 1e0 // Watt [J/s]
	// Pressure units
	s[prefix+"Pa"] = 1e0              // Pascal (pressure) [N/m^2]
	s[prefix+"bar"] = 1e5             // Bar (pressure) [N/m^2]
	s[prefix+"atm"] = 1.01325e5       // Atmospheres (pressure) [N/m^2]
	s[prefix+"torr"] = 1.333224e2     // Torr (pressure) [N/m^2]
	s[prefix+"psi"] = 6.89475729317e3 // Pounds/in^2 [N/m^2]
	s[prefix+"mHg"] = 1.333224e5      // Meter of Mercury [N/m^2]
	// Temperature units
	s[prefix+"degK"] = 1.0e0           // Kelvin [K]
	s[prefix+"degC"] = 1.0e0           // Celsius [K]
	s[prefix+"degF"] = 0.55555555556e0 // Fahrenheit [K]
	// Light units
	s[prefix+"cd"] = 1e0              // Candela [cd]
	s[prefix+"sb"] = 1e4              // Stilb [cd/m^2]
	s[prefix+"lm"] = 1e0              // Lumen [cd sr]
	s[prefix+"lx"] = 1e0              // Lux [cd sr/m^2]
	s[prefix+"ph"] = 1e4              // Phot [lx]
	s[prefix+"lam"] = 3.18309886184e3 // Lambert [cd/m^2]
	// Radiation units
	s[prefix+"Bq"] = 1.0e0   // Becquerel [1/s]
	s[prefix+"Ci"] = 3.7e10  // Curie [1/s]
	s[prefix+"Gy"] = 1.0e0   // Gray [J/kg]
	s[prefix+"Sv"] = 1.0e0   // Sievert [J/kg]
	s[prefix+"R"] = 2.58e-4  // Roentgen [C/kg]
	s[prefix+"rd"] = 1.0e-2  // Rad (radiation) [J/kg]
	s[prefix+"rem"] = 1.0e-2 // Rem [J/kg]
	// Amount of matter units"
	s[prefix+"Mol"] = 1.0e0 // Mol (SI base unit) [mol]
	// Friction units"
	s[prefix+"Poi"] = 1.0e-1 // Poise [kg/m s]
	s[prefix+"St"] = 1.0e-4  // Stokes [m^2/s]
	// Electrical units"
	// Note: units refer to esu, not emu units....
	s[prefix+"Amp"] = 1.0e0          // Ampere [A]
	s[prefix+"C"] = 1.0e0            // Coulomb [C]
	s[prefix+"Fdy"] = 9.6485341472e4 // Faraday [C]
	s[prefix+"volt"] = 1.0e0         // Volt [W/A]
	s[prefix+"ohm"] = 1.0e0          // Ohm [V/A]
	s[prefix+"mho"] = 1.0e0          // Mho [A/V]
	s[prefix+"siemens"] = 1.0e0      // Siemens [A/V]
	s[prefix+"farad"] = 1.0e0        // Farad [C/V]
	s[prefix+"henry"] = 1.0e0        // Henry [Wb/A]
	s[prefix+"T"] = 1.0e0            // Tesla [Wb/m^2]
	s[prefix+"gauss"] = 1.0e-4       // Gauss [Wb/m^2]
	s[prefix+"Wb"] = 1.0e0           // Weber [V s]
	// Angular units
	s[prefix+"rad"] = 1.0e0                 // Radian [rad]
	s[prefix+"sr"] = 1.0e0                  // Steradian [sr]
	s[prefix+"deg"] = 1.745329252e-2        // Degrees [rad]
	s[prefix+"grad"] = 1.570796327e-2       // Grad [rad]
	s[prefix+"arcmin"] = 2.908882087e-4     // Arcminutes [rad]
	s[prefix+"arcsec"] = 4.848136812e-6     // Arcseconds [rad]
	s[prefix+"deg2"] = 3.04617419786e-4     // Square Degrees [sr]
	s[prefix+"arcmin2"] = 8.46159499406e-8  // Square Arcminutes [sr]
	s[prefix+"arcsec2"] = 2.35044305389e-11 // Square Arcseconds [sr]
	// Astronomical Units
	s[prefix+"lyr"] = 9.460536207e15 // Lightyear [m]
	// 1 pc       = AU / arcsec
	s[prefix+"pc"] = 3.085677582e16   // Parsec [m]
	s[prefix+"Lsun"] = 3.82e26        // Solar Luminosity [W]
	s[prefix+"Msun"] = 1.989e30       // Solar Mass [kg]
	s[prefix+"Mjup"] = 1.8986e27      // Jupiter mass [kg]
	s[prefix+"Mea"] = 5.976e24        // Earth Mass [kg]
	s[prefix+"Mmn"] = 7.35e22         // Moon mass [kg]
	s[prefix+"Rsun"] = 6.96e8         // Solar radius [m]
	s[prefix+"Rearth"] = 6.378e6      // Earth radius [m]
	s[prefix+"Rmoon"] = 1.738e6       // Moon radius [m]
	s[prefix+"Rjup"] = 7.1492e7       // Earth radius [m]
	s[prefix+"AU"] = 1.49597870691e11 // Astronomical unit [m]
	s[prefix+"Dmoon"] = 3.844e8       // Distance Earth-Moon [m]
	//s[prefix + "Djup"]    = 7.78412d11        // Distance Sun-Jupiter [m]
	s[prefix+"Jy"] = 1e-26     // Jansky [W / m^2 Hz]
	s[prefix+"ga"] = 9.80665e0 // Earth acceleration [m/s^2]
	// Special Units
	// Planck units:  These definitions use h, not hbar
	s[prefix+"lpl"] = 4.05083e-35 // Planck length (h) [m]
	s[prefix+"mpl"] = 5.45621e-8  // Planck mass (h) [kg]
	s[prefix+"tpl"] = 1.35121e-43 // Planck time (h) [s]
	// Planck units:  These definitions use hbar, not h
	s[prefix+"lplb"] = 1.61605e-35 // Planck length (hbar) [m]
	s[prefix+"mplb"] = 2.17671e-8  // Planck mass (hbar) [kg]
	s[prefix+"tplb"] = 5.39056e-44 // Planck time (hbar) [s]
	return s
}

type TemplateManager struct {
	TemplatePath string
}

// func fuzzyAge(start string) (string, error) {
func fuzzyAge(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	t, err := time.Parse("2006-01-02T15:04:05.000-0700", in.String())
	if err != nil {
		return pongo2.AsValue(""), &pongo2.Error{
			Sender:    "filter:age",
			OrigError: err}
	}
	delta := time.Since(t)
	if delta.Minutes() < 2 {
		return pongo2.AsValue("a minute"), nil
	} else if dm := delta.Minutes(); dm < 45 {
		return pongo2.AsValue(fmt.Sprintf("%d minutes", int(dm))), nil
	} else if dm := delta.Minutes(); dm < 90 {
		return pongo2.AsValue("an hour"), nil
	} else if dh := delta.Hours(); dh < 24 {
		return pongo2.AsValue(fmt.Sprintf("%d hours", int(dh))), nil
	} else if dh := delta.Hours(); dh < 48 {
		return pongo2.AsValue("a day"), nil
	}
	return pongo2.AsValue(fmt.Sprintf("%d days", int(delta.Hours()/24))), nil
}

func cleanupString(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil {
		s := in.String()
		if s != "" {
			s = strings.TrimSpace(strings.ReplaceAll(s, "\u00a0", "\n"))
			return pongo2.AsValue(s), nil
		}
	}
	return pongo2.AsValue(""), nil
}

func wordWrap(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil {
		s := in.String()
		if s != "" {
			s = strings.ReplaceAll(s, "\u00a0", "\n")
			limit := param.Integer()
			s = wordwrap.String(s, limit)
			return pongo2.AsValue(s), nil
		}
	}
	return pongo2.AsValue(""), nil
}

func templateIndent(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil {
		output := in.String()
		if output != "" {
			lvl := param.Integer()
			if lvl < 1 {
				lvl = 1
			}
			idnt := strings.Repeat(" ", lvl+2)
			output = textwrap.Dedent(output)
			output = textwrap.Indent(output, idnt, nil)
			return pongo2.AsValue(output), nil
		}
	}
	return pongo2.AsValue(""), nil
}

// Pad out a number to fit in a space.
func fit(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil {
		output := in.String()
		if output != "" {
			size := param.Integer()
			output = fmt.Sprintf(fmt.Sprintf("%%-%d.%ds", size, size), output)
			return pongo2.AsValue(output), nil
		}
	}
	return pongo2.AsValue(""), nil
}

func orgTrim(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil {
		output := in.String()
		if output != "" {
			output = strings.TrimSpace(output)
			return pongo2.AsValue(output), nil
		}
	}
	return in, nil
}

func dateFormat(format string, content string) (string, error) {
	t, err := time.Parse("2006-01-02T15:04:05.000-0700", content)
	if err != nil {
		return "", err
	}
	return t.Format(format), nil
}

// Provide access to environment variables from within a template
func orgEnv() map[string]string {
	out := map[string]string{}
	for _, env := range os.Environ() {
		kv := strings.SplitN(env, "=", 2)
		out[kv[0]] = kv[1]
	}
	return out
}

func commaList(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil {
		inVar, ok := in.Interface().([]string)
		if ok && inVar != nil {
			output := ""
			for _, v := range inVar {
				v := strings.TrimSpace(v)
				if output != "" {
					output += ", "
				}
				output += v
			}
			output = strings.TrimSpace(output)
			return pongo2.AsValue(output), nil
		}
	}
	return in, nil
}

func sepList(in *pongo2.Value, param *pongo2.Value) (out *pongo2.Value, errOut *pongo2.Error) {
	if in != nil {
		inVar, ok := in.Interface().([]string)
		sep := param.String()
		if ok && inVar != nil {
			output := ""
			for _, v := range inVar {
				v := strings.TrimSpace(v)
				if output != "" {
					output += " " + sep + " "
				}
				output += v
			}
			output = strings.TrimSpace(output)
			return pongo2.AsValue(output), nil
		}
	}
	return in, nil
}

func (self *TemplateManager) Initialize() {
	pongo2.RegisterFilter("age", fuzzyAge)
	pongo2.RegisterFilter("fit", fit)
	pongo2.RegisterFilter("trim", orgTrim)
	pongo2.RegisterFilter("orgIndent", templateIndent)
	pongo2.RegisterFilter("orgCleanup", cleanupString)
	pongo2.RegisterFilter("orgWordWrap", wordWrap)
	pongo2.RegisterFilter("commaList", commaList)
	pongo2.RegisterFilter("sepList", sepList)
}

func (self *TemplateManager) resolveTemplate(name string, context map[string]interface{}) string {
	if _, err := os.Stat(name); err != nil {
		return err.Error()
	}
	tpl, _ := pongo2.FromFile(name)
	ctx := pongo2.Context{}
	self.standardContext(&ctx)
	for k, v := range context {
		ctx[k] = pongo2.AsSafeValue(v)
	}
	res, _ := tpl.Execute(ctx)
	return res
}

func (self *TemplateManager) AugmentContext(context *pongo2.Context, aug map[string]any, expandTemplates bool) *pongo2.Context {
	// Expand out our query filters into our context
	if len(aug) > 0 {
		for k, v := range aug {
			(*context)[k] = pongo2.AsSafeValue(v)
		}
		if expandTemplates {
			for k, v := range aug {
				// We allow a strict 3 levels deep expansion and that's it!
				if s, ok := v.(string); ok {
					for i := 0; i < 3; i++ {
						r := self.ExecuteTemplateString(s, context)
						if r != s {
							s = r
							(*context)[k] = pongo2.AsSafeValue(r)
						} else {
							break
						}
					}
				}
			}
		}
	}
	return context
}

func (self *TemplateManager) AugmentContextFromStringMap(context *pongo2.Context, aug map[string]string, expandTemplates bool) *pongo2.Context {
	// Expand out our query filters into our context
	if len(aug) > 0 {
		for k, v := range aug {
			(*context)[k] = pongo2.AsSafeValue(v)
		}
		if expandTemplates {
			for k, v := range aug {
				// We allow a strict 3 levels deep expansion and that's it!
				for i := 0; i < 3; i++ {
					r := self.ExecuteTemplateString(v, context)
					if r != v {
						v = r
						(*context)[k] = pongo2.AsSafeValue(v)
					} else {
						break
					}
				}
			}
		}
	}
	return context
}

/*
table := tablewriter.NewWriter(out)
table.SetAutoFormatHeaders(false)
headers := []string{}
cells := [][]string{}
*/
func (self *TemplateManager) standardContext(context *pongo2.Context) {
	dt := time.Now()
	userStr := ""
	username := ""
	username = os.Getenv("USERNAME")
	userStr = username
	/*
		if usr, ok := user.Current(); ok == nil {
			username = usr.Username
			userStr = usr.Name
		}
	*/
	(*context)["weekday"] = dt.Format("Mon")
	(*context)["day"] = fmt.Sprintf("%d", dt.Day())
	(*context)["month"] = fmt.Sprintf("%d", dt.Month())
	(*context)["year"] = fmt.Sprintf("%d", dt.Year())
	(*context)["user"] = userStr
	(*context)["username"] = username
	(*context)["date"] = dt.Format("2006-02-01")
	(*context)["datetime"] = dt.Format("2006-02-01 Mon 15:04")
	(*context)["env"] = orgEnv
}

func (self *TemplateManager) GetStandardContext() *pongo2.Context {
	ctx := pongo2.Context{}
	self.standardContext(&ctx)
	return &ctx
}

func (self *TemplateManager) GetAugmentedStandardContext(aug map[string]any, expandTemplates bool) *pongo2.Context {
	return self.AugmentContext(self.GetStandardContext(), aug, expandTemplates)
}

func (self *TemplateManager) GetAugmentedStandardContextFromStringMap(aug map[string]string, expandTemplates bool) *pongo2.Context {
	return self.AugmentContextFromStringMap(self.GetStandardContext(), aug, expandTemplates)
}

func (self *TemplateManager) CompileTemplate(template string) *pongo2.Template {
	tpl, _ := pongo2.FromString(template)
	return tpl
}

func (self *TemplateManager) ExecuteTemplateString(template string, context *pongo2.Context) string {
	tpl, _ := pongo2.FromString(template)
	res, _ := tpl.Execute(*context)
	return res
}

func (self *TemplateManager) resolveTemplateString(template string, context map[string]any) string {
	ctx := self.AugmentContext(self.GetStandardContext(), context, false)
	return self.ExecuteTemplateString(template, ctx)
}

func (self *TemplateManager) ExpandTemplatePath(name string) string {
	tempName, _ := filepath.Abs(name)
	tempFolderName, _ := filepath.Abs(path.Join(self.TemplatePath, name))
	//if _, err := os.Stat(name); err == nil {
	// Use name
	//}
	if _, err := os.Stat(tempName); err == nil {
		// Try abs name of name
		name = tempName
	} else if _, err := os.Stat(tempFolderName); err == nil {
		// Try in the template folder for name
		name = tempFolderName
	}
	return name
}

func (self *TemplateManager) RenderTemplate(name string, context map[string]interface{}) string {
	name = self.ExpandTemplatePath(name)
	return self.resolveTemplate(name, context)
}

func (self *TemplateManager) RenderTemplateString(template string, context map[string]any) string {
	return self.resolveTemplateString(template, context)
}
