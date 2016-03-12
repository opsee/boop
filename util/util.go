package util

import (
	"github.com/opsee/basic/schema"
	"github.com/opsee/boop/errors"
	"github.com/opsee/boop/svc"
	"regexp"
	"time"
)

const uuidFormat = `^[a-z0-9]{8}-[a-z0-9]{4}-[1-5][a-z0-9]{3}-[a-z0-9]{4}-[a-z0-9]{12}$`
const emailFormat = `^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`

func RoundDuration(d, r time.Duration) time.Duration {
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

func GetUserFromArgs(args []string, pos int, svcs *svc.OpseeServices) (*schema.User, error) {
	if len(args) < pos+1 {
		return nil, errors.NewUserError("missing user argument")
	}

	email, uuid, err := parseUserID(args[pos])
	if err != nil {
		return nil, err
	}

	return svcs.GetUser(email, uuid)
}

func GetUUIDFromArgs(args []string, pos int) (*string, error) {
	if len(args) < pos+1 {
		return nil, errors.NewUserError("missing UUID argument")
	}

	uuidExp := regexp.MustCompile(uuidFormat)
	if !uuidExp.MatchString(args[pos]) {
		return nil, errors.NewUserErrorF("invalid UUID: %s", args[pos])
	}

	return &args[pos], nil
}
