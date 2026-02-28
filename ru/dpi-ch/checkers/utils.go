package checkers

import (
	"crypto/sha1"
	"fmt"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Hashes data in SHA1 and returns the first len chars from hex.
func hashData(data string, len int) string {
	h := fmt.Sprintf("%X", sha1.Sum([]byte(data)))
	if len > 20 {
		return h
	}
	return h[:len]
}

// Strips the host name down to k characters
func stripHostToN(host string, k int) string {
	if k <= 0 {
		return ""
	}

	s := essenceHost(host)
	s = stripVowels(s, k)

	r := []rune(s)
	n := len(r)

	if n == 0 {
		return ""
	}
	if n <= k {
		return s
	}
	if k == 1 {
		return string(r[0])
	}

	out := make([]rune, k)
	for i := range k {
		idx := i * (n - 1) / (k - 1)
		out[i] = r[idx]
	}

	return string(out)
}

// Returns the "essence" of the host. Whatever that means ;)
func essenceHost(host string) string {
	s := strings.TrimSpace(host)
	s = strings.ToUpper(s)
	s = strings.TrimSuffix(s, ".")

	suffix, _ := publicsuffix.PublicSuffix(s)
	s = strings.TrimSuffix(s, "."+suffix)
	s = strings.TrimPrefix(s, "WWW.")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "-", "")

	return s
}

// Strip vowels from a string (without affecting the first and last characters).
// Stops stripping if the string is shorter than k.
func stripVowels(s string, k int) string {
	r := []rune(s)
	if n := len(r); n <= 2 || n <= k {
		return s
	}

	out := []rune{r[0]}
	for i := 1; i < len(r)-1; i++ {
		c := r[i]
		isVowel := c == 'A' || c == 'E' || c == 'I' || c == 'O' || c == 'U' || c == 'Y'
		remaining := len(out) + (len(r) - i)
		if isVowel && remaining >= k {
			continue
		}
		out = append(out, c)
	}

	return string(append(out, r[len(r)-1]))
}

func countryIsoToFlagEmoji(iso string) string {
	if len(iso) != 2 {
		return ""
	}

	runes := []rune(iso)
	for i := range 2 {
		c := runes[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if c < 'A' || c > 'Z' {
			return ""
		}
		runes[i] = rune(0x1F1E6 + (c - 'A'))
	}

	return string(runes)
}

func stripHolder(h string) string {
	// TODO: Implement this function normally
	h = strings.TrimPrefix(h, "The ")
	// Spaces are important
	r := []string{" GmbH", " LLC", " Corporation", " Company", " S.A.S.", " S.A.", " SAS", " UAB"}
	for _, v := range r {
		h = strings.ReplaceAll(h, v, "")
	}
	return h
}
