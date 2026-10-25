package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLoadResponseLimit(t *testing.T) {
	for _, limit := range []string{"-1", "1048577", "65536", "0"} {
		t.Run(limit, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("ECHO_OBSERVATORY_MAX_RESPONSE_SIZE", limit)
			cfg, err := Load()
			if limit == "-1" || limit == "1048577" {
				require.ErrorContains(t, err, "maximum response size")
				return
			}
			require.NoError(t, err)
			if limit == "0" {
				require.Zero(t, cfg.MaxResponseSize)
			} else {
				require.Equal(t, int64(65536), cfg.MaxResponseSize)
			}
		})
	}
}
