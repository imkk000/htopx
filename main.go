package main

import (
	"flag"
	"log"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/imkk000/htopx/internal/gpu"
	"github.com/imkk000/htopx/internal/sysinfo"
	"github.com/imkk000/htopx/internal/ui"
)

func main() {
	delay := flag.Int("d", 15, "update delay in tenths of seconds")
	tree := flag.Bool("t", false, "start in tree view")
	hideThreads := flag.Bool("H", false, "hide user threads")
	kernel := flag.Bool("K", false, "show kernel threads")
	noGPU := flag.Bool("no-gpu", false, "disable NVIDIA GPU monitoring")
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("htopx: ")

	opts := ui.Options{
		Delay:       time.Duration(max(*delay, 1)) * 100 * time.Millisecond,
		ShowThreads: !*hideThreads,
		ShowKernel:  *kernel,
		Tree:        *tree,
	}
	var monitor *gpu.Monitor
	if !*noGPU {
		m, err := gpu.Open()
		if err != nil {
			log.Println("gpu monitoring disabled:", err)
		}
		monitor = m
	}
	if err := run(monitor, opts); err != nil {
		log.Fatal(err)
	}
}

func run(monitor *gpu.Monitor, opts ui.Options) error {
	defer monitor.Close()
	_, err := tea.NewProgram(ui.New(sysinfo.NewSampler(), monitor, opts)).Run()
	return err
}
