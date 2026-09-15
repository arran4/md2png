package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func normalizeFormat(value string) (string, error) {
	switch strings.ToLower(value) {
	case "png":
		return "png", nil
	case "jpg", "jpeg":
		return "jpeg", nil
	case "gif":
		return "gif", nil
	default:
		return "", fmt.Errorf("unsupported output format %q (expected png, jpeg, or gif)", value)
	}
}

func resolveFormat(outPath string, formatFlag *string) (string, error) {
	var flagFormat string
	var err error
	if formatFlag != nil && *formatFlag != "" {
		flagFormat, err = normalizeFormat(*formatFlag)
		if err != nil {
			return "", err
		}
	}

	if outPath == "-" {
		if flagFormat == "" {
			return "", errors.New("output to stdout requires explicit --format (e.g. png, jpeg, gif)")
		}
		return flagFormat, nil
	}

	ext := filepath.Ext(outPath)
	var extFormat string
	if ext != "" {
		extFormat, err = normalizeFormat(strings.TrimPrefix(ext, "."))
		if err != nil {
			return "", err
		}
	}

	if flagFormat != "" && extFormat != "" && flagFormat != extFormat {
		return "", errors.New("extension and --format disagree")
	}

	if flagFormat != "" {
		return flagFormat, nil
	}
	if extFormat != "" {
		return extFormat, nil
	}

	return "", errors.New("could not determine output format")
}
