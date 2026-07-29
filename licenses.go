package main

type LicenseInfo struct {
	Name string
	URI  string
}

var LicenseMap = map[string]LicenseInfo{
	"CC-BY-4.0": {
		Name: "Creative Commons Attribution 4.0 International",
		URI:  "https://creativecommons.org/licenses/by/4.0/",
	},
	"CC0-1.0": {
		Name: "Creative Commons Zero 1.0 Universal",
		URI:  "https://creativecommons.org/publicdomain/zero/1.0/",
	},
	"CC-BY-SA-4.0": {
		Name: "Creative Commons Attribution-ShareAlike 4.0 International",
		URI:  "https://creativecommons.org/licenses/by-sa/4.0/",
	},
	"CC-BY-NC-4.0": {
		Name: "Creative Commons Attribution-NonCommercial 4.0 International",
		URI:  "https://creativecommons.org/licenses/by-nc/4.0/",
	},
	"CC-BY-ND-4.0": {
		Name: "Creative Commons Attribution-NoDerivatives 4.0 International",
		URI:  "https://creativecommons.org/licenses/by-nd/4.0/",
	},
	"ODC-By-1.0": {
		Name: "Open Data Commons Attribution License",
		URI:  "https://opendatacommons.org/licenses/by/1-0/",
	},
	"ODbL-1.0": {
		Name: "Open Database License",
		URI:  "https://opendatacommons.org/licenses/odbl/1-0/",
	},
	"PDDL-1.0": {
		Name: "Public Domain Dedication and License",
		URI:  "https://opendatacommons.org/licenses/pddl/1-0/",
	},
}
