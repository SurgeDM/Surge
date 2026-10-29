//go:build !linux && !darwin && !windows

package utils

import "errors"

func renameNoReplace(_, _ string) error { return errors.ErrUnsupported }
