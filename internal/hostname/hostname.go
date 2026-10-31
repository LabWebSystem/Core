package hostname

import "strings"

// Prefix flattens all base-domain labels before the final suffix into one label.
// For example, "lab.home.local" becomes "lab-home.local".
func Prefix(baseDomain string) string {
	labels := strings.Split(strings.TrimSuffix(baseDomain, "."), ".")
	if len(labels) < 2 {
		return baseDomain
	}
	return strings.Join(labels[:len(labels)-1], "-") + "." + labels[len(labels)-1]
}

// Public returns the single-label mDNS hostname for a public application.
func Public(appName, baseDomain string) string {
	return appName + "-" + Prefix(baseDomain)
}
