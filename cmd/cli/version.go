package cli

import (
	"fmt"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: common.I18n("显示版本信息", "Show version information"),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("FlashMemory v%s\n", Version)
	},
}
