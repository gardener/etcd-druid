package compactionlease

import (
	"flag"
	"time"
)

const (
	defaultEnableBackupCompaction = false
	defaultCompactionWorkers      = 3
	defaultEventsThreshold        = 1000000
	defaultActiveDeadlineDuration = 3 * time.Hour
)

// Config contains configuration for the compaction lease controller.
type Config struct {
	// EnableBackupCompaction specifies whether backup compaction should be enabled.
	EnableBackupCompaction bool
	// Workers denotes the number of worker threads for the compaction lease controller.
	Workers int
	// EventsThreshold denotes total number of etcd events to be reached upon which a backup compaction job is triggered.
	EventsThreshold int64
	// ActiveDeadlineDuration is the duration after which a running compaction job will be killed.
	ActiveDeadlineDuration time.Duration
}

func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	flag.BoolVar(&cfg.EnableBackupCompaction, "enable-backup-compaction", defaultEnableBackupCompaction,
		"Enable automatic compaction of etcd backups.")
	flag.IntVar(&cfg.Workers, "compaction-workers", defaultCompactionWorkers,
		"Number of worker threads of the CompactionJob controller. The controller creates a backup compaction job if a certain etcd event threshold is reached. Setting this flag to 0 disabled the controller.")
	flag.Int64Var(&cfg.EventsThreshold, "etcd-events-threshold", defaultEventsThreshold,
		"Total number of etcd events that can be allowed before a backup compaction job is triggered.")
	flag.DurationVar(&cfg.ActiveDeadlineDuration, "active-deadline-duration", defaultActiveDeadlineDuration,
		"Duration after which a running backup compaction job will be killed (Ex: \"300ms\", \"20s\", \"-1.5h\" or \"2h45m\").\").")
}
