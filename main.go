package main

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	
	gonet "github.com/shirou/gopsutil/v3/net"
)

var proxy ProxyServer

// Struttura per tracciare lo stato delle NIC nella GUI
type NICRow struct {
	Name      string
	IP        string
	Check     *widget.Check
	Slider    *widget.Slider
	ValueLbl  *widget.Label
	
	UpLbl     *widget.Label
	DownLbl   *widget.Label
	Graph     *MiniGraph
	PrevSent  uint64
	PrevRecv  uint64
}

func main() {
	// ✓ Gestione panic per debug
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("PANIC: %v\n", r)
			time.Sleep(10 * time.Second) // Mantieni finestra aperta
		}
	}()

	a := app.NewWithID("com.dispatch.proxy")
	a.Settings().SetTheme(&MatrixTheme{}) // ✓ Applica tema Matrix
	w := a.NewWindow("Go Dispatch Proxy - Unified")
	w.Resize(fyne.NewSize(1100, 700))

	// --- Left Panel Components ---
	hostEntry := widget.NewEntry()
	hostEntry.SetText("127.0.0.1")
	portEntry := widget.NewEntry()
	portEntry.SetText("8080")
	tunnelCheck := widget.NewCheck("Tunnel Mode", nil)
	quietCheck := widget.NewCheck("Quiet Mode", nil)

	nicContainer := container.NewVBox()
	var nicRows = make(map[string]*NICRow)
	var nicMutex sync.RWMutex // ✓ Protezione concorrenza

	refreshNICs := func() {
		nicMutex.Lock()
		defer nicMutex.Unlock()
		
		nicContainer.Objects = nil
		loadedNICs := getValidInterfaces()
		
		sort.Slice(loadedNICs, func(i, j int) bool { return loadedNICs[i].ip < loadedNICs[j].ip })

		for _, nic := range loadedNICs {
			lbl := widget.NewLabel(fmt.Sprintf("%s (%s)", nic.ip, nic.name))
			chk := widget.NewCheck("", nil)
			sl := widget.NewSlider(1, 4)
			sl.Step = 1
			sl.Value = 1
			valLbl := widget.NewLabel("1")
			
			// Mantieni stato se esisteva
			if old, ok := nicRows[nic.ip]; ok {
				chk.Checked = old.Check.Checked
				sl.Value = old.Slider.Value
				valLbl.SetText(old.ValueLbl.Text)
			}

			sl.OnChanged = func(v float64) { valLbl.SetText(fmt.Sprintf("%d", int(v))) }

			upL := widget.NewLabel("0.0")
			downL := widget.NewLabel("0.0")
			gr := NewMiniGraph(theme.PrimaryColor())
			
			row := &NICRow{
				Name: nic.name, IP: nic.ip, Check: chk, Slider: sl, ValueLbl: valLbl,
				UpLbl: upL, DownLbl: downL, Graph: gr,
			}
			nicRows[nic.ip] = row

			// Layout riga selezione con slider più visibile
			sliderContainer := container.NewHBox(
				widget.NewLabel("Weight:"),
				sl,
				valLbl,
			)
			topRow := container.NewBorder(nil, nil, chk, sliderContainer, lbl)
			nicContainer.Add(topRow)
		}
		nicContainer.Refresh()
	}

	refreshBtn := widget.NewButton("Refresh Interfaces", refreshNICs)
	
	// ✓ Status indicator
	statusLabel := widget.NewLabel("■ Proxy: Stopped")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	
	startBtn := widget.NewButton("Start Proxy", nil)

	// --- Right Panel Components ---
	logArea := widget.NewMultiLineEntry()
	logArea.TextStyle = fyne.TextStyle{Monospace: true}
	logArea.Wrapping = fyne.TextWrapBreak
	logArea.Disable()
	
	// ✓ Tema Matrix verde fosforescente
	logArea.OnCursorChanged = func() {} // Mantiene scroll in basso
	
	var logMutex sync.Mutex
	const maxLogLines = 1000 // ✓ Limita righe per performance
	
	logger := func(msg string) {
		logMutex.Lock()
		defer logMutex.Unlock()
		
		if quietCheck.Checked && strings.Contains(msg, "[DEBUG]") { return }
		
		// ✓ Limita numero di righe
		lines := strings.Split(logArea.Text, "\n")
		if len(lines) > maxLogLines {
			lines = lines[len(lines)-maxLogLines:]
			logArea.SetText(strings.Join(lines, "\n"))
		}
		
		logArea.SetText(logArea.Text + msg + "\n")
		logArea.CursorRow = len(strings.Split(logArea.Text, "\n"))
		logArea.Refresh()
	}

	// --- Stats Grid ---
	statsContainer := container.NewVBox()
	statsScroll := container.NewVScroll(statsContainer)
	statsScroll.SetMinSize(fyne.NewSize(0, 200)) // ✓ Altezza minima per stats
	
	updateStats := func() {
		nicMutex.RLock()
		defer nicMutex.RUnlock()
		
		statsContainer.Objects = nil
		statsContainer.Add(container.NewGridWithColumns(4, 
			widget.NewLabelWithStyle("Interface", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Upload (Mb/s)", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Download (Mb/s)", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Activity", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		))
		
		counters, err := gonet.IOCounters(true)
		if err != nil {
			return // ✓ Gestione errore silente
		}
		
		counterMap := make(map[string]gonet.IOCountersStat)
		for _, c := range counters { counterMap[c.Name] = c }

		ips := make([]string, 0, len(nicRows))
		for ip := range nicRows { ips = append(ips, ip) }
		sort.Strings(ips)

		for _, ip := range ips {
			row := nicRows[ip]
			stat, exists := counterMap[row.Name]
			if !exists { continue }

			var upRate, downRate float64
			if row.PrevSent > 0 {
				upRate = float64(stat.BytesSent - row.PrevSent) * 8 / 1_000_000
				downRate = float64(stat.BytesRecv - row.PrevRecv) * 8 / 1_000_000
			}
			row.PrevSent = stat.BytesSent
			row.PrevRecv = stat.BytesRecv
			
			// ✓ Formattazione migliorata con colori
			row.UpLbl.SetText(fmt.Sprintf("%.2f", upRate))
			row.DownLbl.SetText(fmt.Sprintf("%.2f", downRate))
			row.Graph.AddValue(downRate + upRate) // ✓ Totale traffico

			// ✓ Highlight interfaccia attiva
			nameLabel := widget.NewLabel(fmt.Sprintf("%s (%s)", row.IP, row.Name))
			if row.Check.Checked {
				nameLabel = widget.NewLabelWithStyle(
					fmt.Sprintf("▶ %s (%s)", row.IP, row.Name),
					fyne.TextAlignLeading,
					fyne.TextStyle{Bold: true},
				)
			}

			statsContainer.Add(container.NewGridWithColumns(4,
				nameLabel,
				row.UpLbl,
				row.DownLbl,
				container.NewPadded(row.Graph),
			))
		}
		statsContainer.Refresh()
	}

	// ✓ Loop aggiornamento stats con context
	stopStats := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				updateStats()
			case <-stopStats:
				return
			}
		}
	}()

	// Start Logic
	startBtn.OnTapped = func() {
		if proxy.running {
			proxy.Stop()
			startBtn.SetText("Start Proxy")
			startBtn.Importance = widget.MediumImportance
			statusLabel.SetText("■ Proxy: Stopped")
			return
		}

		// ✓ Lock per lettura sicura
		nicMutex.RLock()
		var selected []string
		for ip, row := range nicRows {
			if row.Check.Checked {
				w := int(row.Slider.Value)
				if w > 1 {
					selected = append(selected, fmt.Sprintf("%s@%d", ip, w))
				} else {
					selected = append(selected, ip)
				}
			}
		}
		nicMutex.RUnlock()

		if len(selected) == 0 {
			// ✓ Dialog corretto
			dialog.ShowInformation("Error", "Please select at least one interface", w)
			return
		}

		port, err := strconv.Atoi(portEntry.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("invalid port: %v", err), w)
			return
		}
		
		logger("--- Starting Proxy ---")
		err = proxy.Start(hostEntry.Text, port, tunnelCheck.Checked, selected, logger)
		if err != nil {
			logger(fmt.Sprintf("[ERROR] %v", err))
			dialog.ShowError(err, w)
			statusLabel.SetText("■ Proxy: Error")
		} else {
			startBtn.SetText("Stop Proxy")
			startBtn.Importance = widget.HighImportance
			statusLabel.SetText("▶ Proxy: Running")
		}
	}

	// ✓ Cleanup al chiusura
	w.SetOnClosed(func() {
		close(stopStats)
		if proxy.running {
			proxy.Stop()
		}
	})

	// Init
	refreshNICs()

	// Layout Finale
	leftPanel := container.NewVBox(
		widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("Host", hostEntry),
			widget.NewFormItem("Port", portEntry),
		),
		tunnelCheck,
		quietCheck,
		layout.NewSpacer(),
		widget.NewLabelWithStyle("Interfaces", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		refreshBtn,
		container.NewVScroll(nicContainer),
		layout.NewSpacer(),
		statusLabel, // ✓ Status prima del bottone
		startBtn,
	)
	
	rightPanel := container.NewVSplit(
		container.NewBorder(widget.NewLabel("Logs"), nil, nil, nil, logArea),
		container.NewBorder(widget.NewLabel("Real-time Statistics"), nil, nil, nil, container.NewVScroll(statsContainer)),
	)
	rightPanel.SetOffset(0.6)

	content := container.NewBorder(nil, nil, container.NewPadded(leftPanel), nil, rightPanel)
	w.SetContent(content)
	w.ShowAndRun()
}

type nicInfo struct { ip, name string }

func getValidInterfaces() []nicInfo {
	var res []nicInfo
	ifaces, err := net.Interfaces()
	if err != nil {
		return res // ✓ Gestione errore
	}
	
	for _, i := range ifaces {
		if strings.Contains(strings.ToLower(i.Name), "loop") { continue }
		if i.Flags&net.FlagUp == 0 { continue }
		
		addrs, err := i.Addrs()
		if err != nil { continue }
		
		for _, addr := range addrs {
			var ip string
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP.String()
			case *net.IPAddr:
				ip = v.IP.String()
			}

			if strings.Count(ip, ".") == 3 && !strings.HasPrefix(ip, "127.") {
				res = append(res, nicInfo{ip, i.Name})
			}
		}
	}
	return res
}



