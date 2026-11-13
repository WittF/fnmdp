// Command getv2 performs a single low-frequency getV2 poll and prints the
// normalized screen/SKU table. It is a manual probe — one rate-respecting
// request, no loop. On 429/412 it prints the classification and exits cleanly.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/WittF/fnmdp/shared/getv2"
)

func main() {
	id := flag.Int64("id", 1001701, "bilibili 会员购 project id")
	timeout := flag.Duration("timeout", 15*time.Second, "request timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := getv2.New(nil)
	raw, err := client.Fetch(ctx, *id)
	if err != nil {
		// Rate control is an expected outcome, not a crash. Classify and exit 0.
		var re *getv2.RateError
		switch {
		case errors.Is(err, getv2.ErrSoftLimited):
			fmt.Printf("⚠ project %d: SOFT LIMITED (HTTP 429) — back off and retry later\n", *id)
			return
		case errors.Is(err, getv2.ErrHardBlocked):
			msg := "⚠ project %d: HARD BLOCKED (HTTP 412) — IP cooldown ~8–9min, stop polling"
			if errors.As(err, &re) && re.SecRequestID != "" {
				msg += fmt.Sprintf(" [x-sec-request-id=%s]", re.SecRequestID)
			}
			fmt.Printf(msg+"\n", *id)
			return
		default:
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	ps := getv2.Normalize(raw)
	printTable(ps)
}

func printTable(ps getv2.ProjectState) {
	fmt.Printf("Project %d  %s\n", ps.ProjectID, ps.Name)
	fmt.Printf("  flag=%s  hot=%t  wish=%d  price ¥%.2f–¥%.2f  serverTs=%d (%s)\n\n",
		flagLabel(ps.Flag), ps.HotProject, ps.WishCount,
		float64(ps.PriceLowFen)/100, float64(ps.PriceHighFen)/100,
		ps.ServerTs, fmtTs(ps.ServerTs))

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "SCREEN\tSKU\tDESC\tFLAG\tCLICKABLE\tPRICE")
	for _, s := range ps.Screens {
		fmt.Fprintf(w, "%d (%s)\t\t\t%s\t%t\t\n", s.ScreenID, s.Name, flagLabel(s.Flag), s.Clickable)
		for _, sku := range s.Skus {
			label := flagLabel(sku.Flag)
			if sku.FlagText != "" {
				label = sku.FlagText
			}
			fmt.Fprintf(w, "\t%d\t%s\t%s\t%t\t¥%.2f\n",
				sku.SkuID, sku.Desc, label, sku.Clickable, float64(sku.PriceFen)/100)
		}
	}
	w.Flush()
}

func flagLabel(flag int) string {
	switch flag {
	case 1:
		return "未开售"
	case 2:
		return "在售"
	case 4:
		return "售罄"
	case 8:
		return "其他"
	default:
		return fmt.Sprintf("?%d", flag)
	}
}

func fmtTs(ts int64) string {
	if ts == 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}
