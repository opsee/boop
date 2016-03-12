package cmd

import (
	"github.com/opsee/boop/errors"
	"regexp"
	"time"
)

const uuidFormat = `^[a-z0-9]{8}-[a-z0-9]{4}-[1-5][a-z0-9]{3}-[a-z0-9]{4}-[a-z0-9]{12}$`
const emailFormat = `^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`

func roundDuration(d, r time.Duration) time.Duration {
	if r <= 0 {
		return d
	}
	neg := d < 0
	if neg {
		d = -d
	}
	if m := d % r; m+m < r {
		d = d - m
	} else {
		d = d + r - m
	}
	if neg {
		return -d
	}
	return d
}

func parseUserID(id string) (email string, uuid string, err error) {
	emailExp := regexp.MustCompile(emailFormat)
	uuidExp := regexp.MustCompile(uuidFormat)

	if emailExp.MatchString(id) {
		return id, "", nil
	}

	if uuidExp.MatchString(id) {
		return "", id, nil
	}

	return "", "", errors.NewUserError("no email or UUID found in string")
}
