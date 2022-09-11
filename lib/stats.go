// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"fmt"
)

// DeployStats contains some simple stats about the deployment.
type DeployStats struct {
	// Number of files deleted.
	Deleted uint64
	// Number of files on remote not present locally (-max-delete threshold reached)
	Stale uint64
	// Number of files uploaded.
	Uploaded uint64
	// Number of files skipped (i.e. not changed)
	Skipped uint64
}

// Summary returns formatted summary of the stats.
func (d DeployStats) Summary() string {
	return fmt.Sprintf("Deleted %d of %d, uploaded %d, skipped %d (%.0f%% changed)", d.Deleted, (d.Deleted + d.Stale), d.Uploaded, d.Skipped, d.PercentageChanged())
}

// FileCountChanged returns the total number of files changed on server.
func (d DeployStats) FileCountChanged() uint64 {
	return d.Deleted + d.Uploaded
}

// FileCount returns the total number of files both locally and remote.
func (d DeployStats) FileCount() uint64 {
	return d.FileCountChanged() + d.Skipped
}

// PercentageChanged returns the percentage of files that have changed.
func (d DeployStats) PercentageChanged() float32 {
	if d.FileCount() == 0 {
		return 0.0
	}
	return (float32(d.FileCountChanged()) / float32(d.FileCount()) * 100)
}
