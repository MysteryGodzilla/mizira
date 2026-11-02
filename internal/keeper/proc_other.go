// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

//go:build !windows

package keeper

import "os/exec"

func hide(*exec.Cmd) {}
