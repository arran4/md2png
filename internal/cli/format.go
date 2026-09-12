package cli

import (
	"errors"
	"path/filepath"
	"strings"
)

func resolveFormat(outPath string, formatFlag *string) (string, error) {
	var flagFormat string
	if formatFlag != nil {
		flagFormat = strings.ToLower(*formatFlag)
	}

	if outPath == "-" {
		if flagFormat == "" {
			return "", errors.New("output to stdout requires explicit --format (e.g. png, jpeg, gif)")
		}
		return flagFormat, nil
	}

	ext := strings.ToLower(filepath.Ext(outPath))
	extFormat := ""
	if ext != "" {
		extFormat = ext[1:] // remove leading dot
		if extFormat == "jpg" {
			extFormat = "jpeg"
		}
	}

	if flagFormat != "" && extFormat != "" {
		if flagFormat != extFormat {
			return "", errors.New("extension and --format disagree")
		}
	}

	if flagFormat != "" {
		return flagFormat, nil
	}
	if extFormat != "" {
		return extFormat, nil
	}

	return "", errors.New("could not determine output format")
}
