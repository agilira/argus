package argus

import (
	"testing"
)

func TestFormatStrings(t *testing.T) {
	formats := []ConfigFormat{FormatJSON, FormatYAML, FormatTOML, FormatHCL, FormatINI, FormatProperties}

	for _, format := range formats {
		t.Logf("Format: %d, String(): %s", format, format.String())
	}
}
