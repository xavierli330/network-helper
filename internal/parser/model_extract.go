package parser

import (
	"regexp"
	"strings"
)

// modelPattern pairs a compiled regex with a canonical model prefix.
// The first captured group is used as the model string.
type modelPattern struct {
	re *regexp.Regexp
}

// vendorModelPatterns maps vendor → list of hostname-based model extraction patterns.
// Order matters: first match wins.
var vendorModelPatterns = map[string][]modelPattern{
	"huawei": {
		{re: regexp.MustCompile(`(?i)\b(NE\d{2,5}[A-Z]*)\b`)},       // NE40E, NE8000, NE5000E
		{re: regexp.MustCompile(`(?i)\b(CE\d{3,5}[A-Z]*)\b`)},       // CE12800, CE6800
		{re: regexp.MustCompile(`(?i)[-_](HW\d{3,5}[A-Z]*)\b`)},     // HW12816 (hostname convention: HW=Huawei)
		{re: regexp.MustCompile(`(?i)\b(S\d{3,5}[A-Z]*)\b`)},        // S12700, S5700
		{re: regexp.MustCompile(`(?i)\b(AR\d{3,5}[A-Z]*)\b`)},       // AR2200, AR1200
		{re: regexp.MustCompile(`(?i)\b(ATN\d{3,5}[A-Z]*)\b`)},      // ATN950B, ATN910
		{re: regexp.MustCompile(`(?i)\b(CX\d{3,5}[A-Z]*)\b`)},       // CX600
		{re: regexp.MustCompile(`(?i)\b(USG\d{3,5}[A-Z]*)\b`)},      // USG6000
		{re: regexp.MustCompile(`(?i)\b(NetEngine\d+[A-Za-z]*)\b`)},  // NetEngine8000
	},
	"h3c": {
		{re: regexp.MustCompile(`(?i)\b(H\d{4,5}[A-Z0-9]*)\b`)},     // H12508AF, H12516AF, H12504AF, H9850C, H6800QT, H6800QTH3
		{re: regexp.MustCompile(`(?i)\b(SR\d{3,5}[A-Z]*)\b`)},        // SR8800, SR6600
		{re: regexp.MustCompile(`(?i)\b(CR\d{3,5}[A-Z]*)\b`)},        // CR19000
		{re: regexp.MustCompile(`(?i)\b(S\d{3,5}[A-Z]*)\b`)},         // S12500, S9850, S6800
		{re: regexp.MustCompile(`(?i)\b(MSR\d{3,5}[A-Z]*)\b`)},       // MSR3600
		{re: regexp.MustCompile(`(?i)\b(SecPath\s*\w+)\b`)},           // SecPath F100
	},
	"cisco": {
		{re: regexp.MustCompile(`(?i)\b(ASR\d{3,5}[A-Z]*)\b`)},       // ASR9000, ASR1000, ASR9912
		{re: regexp.MustCompile(`(?i)\b(NCS\d{3,5}[A-Z]*)\b`)},       // NCS5500, NCS540
		{re: regexp.MustCompile(`(?i)\b(C\d{3,5}[A-Z]*)\b`)},         // C8200, C3850
		{re: regexp.MustCompile(`(?i)\b(ISR\d{3,5}[A-Z]*)\b`)},       // ISR4000
		{re: regexp.MustCompile(`(?i)\b(N9K[A-Z0-9-]*)\b`)},          // N9K-C9300
		{re: regexp.MustCompile(`(?i)\b(N7K[A-Z0-9-]*)\b`)},          // N7K-C7000
		{re: regexp.MustCompile(`(?i)\b(XR[Vv]\d*)\b`)},              // XRv9000
	},
	"juniper": {
		{re: regexp.MustCompile(`(?i)\b(MX\d{2,5}[A-Z]*)\b`)},        // MX960, MX480, MX204
		{re: regexp.MustCompile(`(?i)\b(QFX\d{3,5}[A-Z]*)\b`)},       // QFX5100, QFX10002
		{re: regexp.MustCompile(`(?i)\b(EX\d{3,5}[A-Z]*)\b`)},        // EX4300, EX9200
		{re: regexp.MustCompile(`(?i)\b(SRX\d{3,5}[A-Z]*)\b`)},       // SRX4600
		{re: regexp.MustCompile(`(?i)\b(PTX\d{3,5}[A-Z]*)\b`)},       // PTX10000
		{re: regexp.MustCompile(`(?i)\b(ACX\d{3,5}[A-Z]*)\b`)},       // ACX5000
		{re: regexp.MustCompile(`(?i)\b(vMX|vSRX|vQFX)\b`)},          // virtual models
	},
}

// ExtractModel attempts to extract a device model string from a hostname.
// It first tries vendor-specific patterns, then falls back to trying all
// vendors' patterns.
// Returns an uppercase model string, or "" if no match.
func ExtractModel(hostname, vendor string) string {
	upper := strings.ToUpper(hostname)

	// First: try patterns for the specified vendor.
	if patterns, ok := vendorModelPatterns[vendor]; ok {
		for _, mp := range patterns {
			if m := mp.re.FindStringSubmatch(upper); m != nil {
				return strings.ToUpper(m[1])
			}
		}
	}

	// Fallback: try all other vendors' patterns.
	for v, patterns := range vendorModelPatterns {
		if v == vendor {
			continue
		}
		for _, mp := range patterns {
			if m := mp.re.FindStringSubmatch(upper); m != nil {
				return strings.ToUpper(m[1])
			}
		}
	}

	return ""
}
