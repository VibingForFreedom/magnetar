package banning

import (
	"errors"

	"github.com/magnetar/magnetar/internal/crawler/metainfo"
)

type nameLengthChecker struct {
	min int
}

func (c nameLengthChecker) Check(info metainfo.Info) error {
	if len(info.BestName()) < c.min {
		return errors.New("name too short")
	}
	return nil
}
