package system

import (
	"fmt"
	"os/exec"
	"regexp"
)

var validUsername = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// CreateAccount creates a Linux system user for a hosting account.
func CreateAccount(username string, uid uint32, homedir string, quotaBytes uint64) error {
	if !validUsername.MatchString(username) {
		return fmt.Errorf("invalid username %q", username)
	}

	args := []string{"-m", "-s", "/bin/bash"}
	if homedir != "" {
		args = append(args, "-d", homedir)
	}
	if uid > 0 {
		args = append(args, "-u", fmt.Sprintf("%d", uid))
	}
	args = append(args, username)

	if out, err := exec.Command("useradd", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("useradd: %w: %s", err, out)
	}

	if quotaBytes > 0 {
		if err := setQuota(username, quotaBytes); err != nil {
			// Non-fatal — quota tooling may not be installed
			fmt.Printf("warning: setquota for %s: %v\n", username, err)
		}
	}

	return nil
}

// DeleteAccount removes a Linux system user.
func DeleteAccount(username string, purgeFiles bool) error {
	if !validUsername.MatchString(username) {
		return fmt.Errorf("invalid username %q", username)
	}

	args := []string{}
	if purgeFiles {
		args = append(args, "-r")
	}
	args = append(args, username)

	if out, err := exec.Command("userdel", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("userdel: %w: %s", err, out)
	}

	return nil
}

func setQuota(username string, quotaBytes uint64) error {
	// setquota -u <user> <soft-blocks> <hard-blocks> <soft-inodes> <hard-inodes> <filesystem>
	// Block size is 1KB for quota purposes.
	softKB := quotaBytes / 1024
	hardKB := softKB + softKB/10 // 10% grace above soft limit

	out, err := exec.Command("setquota",
		"-u", username,
		fmt.Sprintf("%d", softKB),
		fmt.Sprintf("%d", hardKB),
		"0", "0", "/",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}
