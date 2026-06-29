package inetutil

import (
	"strings"

	"golang.org/x/net/publicsuffix"
)

type TlsSanCnItem struct {
	Name   string
	Wild   bool
	Single bool
	Some   bool
}

// An attempt to extract the "primary" domain name from the certificate (using SAN and CN).
// Obviously, this won't work in all cases — especially when there are multiple domains of equal rank.
func TlsSanCn(san []string, cn string) TlsSanCnItem {
	names := tlsSanCnMerge(san, cn)

	var best TlsSanCnItem
	bestRank, bestDepth, bestLen := 0, 0, 0

	for name := range names {
		wild := strings.HasPrefix(name, "*.")
		host := strings.TrimPrefix(name, "*.")

		base, err := publicsuffix.EffectiveTLDPlusOne(host)
		if err != nil {
			continue
		}

		rank := 3
		switch {
		case name == base:
			rank = 0 // site.com
		case name == "*."+base:
			rank = 1 // *.site.com
		case !wild:
			rank = 2 // www.site.com
		}

		depth := strings.Count(host, ".")
		length := len(host)

		if best.Name == "" ||
			rank < bestRank ||
			(rank == bestRank && depth < bestDepth) ||
			(rank == bestRank && depth == bestDepth && length < bestLen) {
			best.Name = host
			bestRank, bestDepth, bestLen = rank, depth, length
		}
	}

	if best.Name == "" {
		return best
	}

	best.Wild = names["*."+best.Name]
	best.Single = names[best.Name]

	if best.Wild && best.Single {
		best.Some = len(names) > 2
	} else {
		best.Some = len(names) > 1
	}

	return best
}

func tlsSanCnMerge(san []string, cn string) map[string]bool {
	names := make(map[string]bool, len(san)+1)

	for _, name := range san {
		names[name] = true
	}

	names[cn] = true
	delete(names, "")

	for name := range names {
		if !strings.HasPrefix(name, "*.") {
			continue
		}

		host := strings.TrimPrefix(name, "*.")

		for other := range names {
			if strings.HasPrefix(other, "*.") || !strings.HasSuffix(other, "."+host) {
				continue
			}

			left := strings.TrimSuffix(other, "."+host)
			if left != "" && !strings.Contains(left, ".") {
				delete(names, other)
			}
		}
	}

	return names
}
