package experiment

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"zdx-ban/internal/measurement"
)

type Dataset struct {
	Version, Path, SHA256 string
	Cases                 []Case
}

func LoadDataset(path string, registry *Registry) (Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return Dataset{}, err
	}
	defer f.Close()
	h := sha256.New()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 4<<20)
	seen := map[string]bool{}
	fingerprints := map[string]string{}
	var d Dataset
	d.Path = path
	line := 0
	for scan.Scan() {
		line++
		raw := append([]byte(nil), scan.Bytes()...)
		h.Write(raw)
		h.Write([]byte{'\n'})
		if strings.TrimSpace(string(raw)) == "" {
			continue
		}
		var c Case
		if err = json.Unmarshal(raw, &c); err != nil {
			return d, fmt.Errorf("line %d: %w", line, err)
		}
		if err = ValidateCase(c, registry); err != nil {
			return d, fmt.Errorf("line %d case %q: %w", line, c.ID, err)
		}
		if seen[c.ID] {
			return d, fmt.Errorf("duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
		fp := strings.ToLower(strings.Join(strings.Fields(c.Prompt), " "))
		if prior, ok := fingerprints[fp]; ok {
			return d, fmt.Errorf("duplicate prompt %q and %q", prior, c.ID)
		}
		fingerprints[fp] = c.ID
		if d.Version == "" {
			d.Version = c.DatasetVersion
		} else if d.Version != c.DatasetVersion {
			return d, fmt.Errorf("mixed dataset versions")
		}
		d.Cases = append(d.Cases, c)
	}
	if err = scan.Err(); err != nil {
		return d, err
	}
	d.SHA256 = hex.EncodeToString(h.Sum(nil))
	for i := range d.Cases {
		d.Cases[i].Measurement.Contract.Provenance.DatasetVersion = d.Version
		d.Cases[i].Measurement.Contract.Provenance.DatasetHash = d.SHA256
	}
	if len(d.Cases) == 0 {
		return d, fmt.Errorf("empty dataset")
	}
	return d, nil
}
func ValidateCase(c Case, r *Registry) error {
	if c.ID == "" || c.Version == "" || c.DatasetVersion == "" || c.Category == "" || strings.TrimSpace(c.Prompt) == "" {
		return fmt.Errorf("missing required field")
	}
	if c.Measurement.VerificationClass == "" {
		return fmt.Errorf("measurement verification class required")
	}
	if err := measurement.ValidateContract(c.Measurement.Contract); err != nil {
		return err
	}
	v, ok := r.Get(c.VerifierType)
	if !ok {
		return fmt.Errorf("unsupported verifier %q", c.VerifierType)
	}
	return v.Validate(c)
}
