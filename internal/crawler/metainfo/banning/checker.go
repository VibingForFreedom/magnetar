// Package banning provides content validation for torrent metadata.
// Extracted from github.com/bitmagnet-io/bitmagnet (MIT License).
package banning

import (
	"errors"

	"github.com/magnetar/magnetar/internal/crawler/metainfo"
)

type Checker interface {
	Check(metainfo.Info) error
}

type combinedChecker struct {
	checkers []Checker
}

func (c combinedChecker) Check(info metainfo.Info) error {
	var errs []error
	for _, checker := range c.checkers {
		if err := checker.Check(info); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// NewChecker creates a combined checker with default validation rules.
func NewChecker() Checker {
	return combinedChecker{
		checkers: []Checker{
			nameLengthChecker{min: 8},
			sizeChecker{min: 1024},
			utf8Checker{},
		},
	}
}
