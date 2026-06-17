package record

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Marshal serializes a Record back to its on-disk markdown form: a YAML
// frontmatter block fenced by "---" lines, then a blank line, then the
// narrative body. It is the inverse of Parse: for any valid record r,
// Parse(r.Path, Marshal(r)) reproduces r (except Raw, the byte encoding).
func Marshal(r *Record) ([]byte, error) {
	frontmatter, err := yaml.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}

	var buf []byte
	buf = append(buf, "---\n"...)
	buf = append(buf, frontmatter...)
	buf = append(buf, "---\n\n"...)
	buf = append(buf, r.Body...)
	buf = append(buf, '\n')

	return buf, nil
}
