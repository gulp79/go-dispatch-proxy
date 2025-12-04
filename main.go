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
	Name     string
	IP       string
	Check    *widget.Check
	Slider   *widget.Slider
	ValueLbl *widget.Label

	// Widget per le statistiche (riutilizzati)
	StatsNameLbl *widget.Label
	UpLbl        *widget.Label
	DownLbl      *widget.Label
	Graph        *MiniGraph
	PrevSent     uint64
	PrevRecv     uint64
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("PANIC: %v\n", r)
		}
	}()

	a := app.NewWithID("com.dispatch.proxy")
	a.Settings().SetTheme(&MatrixTheme{})
	w := a.NewWindow("Go Dispatch Proxy - Unified")
	w.Resize(fyne.NewSize(1100, 700))

	// --- Left Panel Components ---
	hostEntry := widget.NewEntry()
	hostEntry.SetText("127.0.0.1")
	portEntry := widget.NewEntry()
	portEntry.SetText("8080")
	tunnelCheck := widget.NewCheck("Tunnel Mode", nil)
	quietCheck := widget.NewCheck("Quiet Mode", nil)

	nicContainer := container.NewVBox()     // Lista checkbox a sinistra
	statsContainer := container.NewVBox()   // Lista stats a destra
	
	var nicRows = make(map[string]*NICRow)
	var nicMutex sync.RWMutex

	// Funzione per ricostruire l'interfaccia quando si fa "Refresh"
	refreshNICs := func() {
		nicMutex.Lock()
		defer nicMutex.Unlock()

		nicContainer.Objects = nil
		statsContainer.Objects = nil // Pulisce il container stats una sola volta al refresh

		// Intestazione Statistiche (Fissa)
		headerObj := container.NewGridWithColumns(4,
			widget.NewLabelWithStyle("Interface", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Upload (Mb/s)", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),   // Allineato a destra
			widget.NewLabelWithStyle("Download (Mb/s)", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}), // Allineato a destra
			widget.NewLabelWithStyle("Activity", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		)
		statsContainer.Add(headerObj)

		loadedNICs := getValidInterfaces()
		sort.Slice(loadedNICs, func(i, j int) bool { return loadedNICs[i].ip < loadedNICs[j].ip })

		for _, nic := range loadedNICs {
			// --- Componenti Selezione (Sinistra) ---
			lbl := widget.NewLabel(fmt.Sprintf("%s (%s)", nic.ip, nic.name))
			chk := widget.NewCheck("", nil)
			sl := widget.NewSlider(1, 4)
			sl.Step = 1
			sl.Value = 1
			valLbl := widget.NewLabel("1")

			// Ripristina stato precedente se esiste
			if old, ok := nicRows[nic.ip]; ok {
				chk.Checked = old.Check.Checked
				sl.Value = old.Slider.Value
				valLbl.SetText(old.ValueLbl.Text)
			}

			sl.OnChanged = func(v float64) { valLbl.SetText(fmt.Sprintf("%d", int(v))) }

			// --- Componenti Statistiche (Destra - Creati ORA e riutilizzati) ---
			
			// Label Nome Interfaccia
			sName := widget.NewLabel(fmt.Sprintf("%s (%s)", nic.ip, nic.name))
			sName.Truncation = fyne.TextTruncateEllipsis
			
			// Label Upload (Allineata a destra)
			sUp := widget.NewLabel("0.00")
			sUp.Alignment = fyne.TextAlignTrailing // FONDAMENTALE PER ALLINEAMENTO
			
			// Label Download (Allineata a destra)
			sDown := widget.NewLabel("0.00")
			sDown.Alignment = fyne.TextAlignTrailing // FONDAMENTALE PER ALLINEAMENTO

			// Grafico
			gr := NewMiniGraph(theme.PrimaryColor())

			row := &NICRow{
				Name: nic.name, IP: nic.ip, Check: chk, Slider: sl, ValueLbl: valLbl,
				StatsNameLbl: sName, UpLbl: sUp, DownLbl: sDown, Graph: gr,
			}
			nicRows[nic.ip] = row

			// Aggiungi a UI Sinistra
			sliderContainer := container.NewHBox(widget.NewLabel("Weight:"), sl, valLbl)
			topRow := container.NewBorder(nil, nil, chk, sliderContainer, lbl)
			nicContainer.Add(topRow)

			// Aggiungi a UI Destra (Grid statica)
			statsRow := container.NewGridWithColumns(4,
				sName,
				sUp,
				sDown,
				container.NewPadded(gr), // Padding per il grafico
			)
			statsContainer.Add(statsRow)
		}
		
		nicContainer.Refresh()
		statsContainer.Refresh()
	}

	refreshBtn := widget.NewButton("Refresh Interfaces", refreshNICs)
	statusLabel := widget.NewLabel("🔴 Proxy: Stopped")
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	startBtn := widget.NewButton("Start Proxy", nil)

	// --- Log Area Ottimizzata ---
	logArea := widget.NewMultiLineEntry()
	logArea.TextStyle = fyne.TextStyle{Monospace: true}
	logArea.Wrapping = fyne.TextWrapBreak
	logArea.Disable()

	var logMutex sync.Mutex
	// Buffer circolare per i log (evita allocazioni stringa infinite)
	logBuffer := make([]string, 0, 100) 
	const maxLogLines = 100

	logger := func(msg string) {
		logMutex.Lock()
		defer logMutex.Unlock()

		if quietCheck.Checked && strings.Contains(msg, "[DEBUG]") {
			return
		}

		// Aggiungi al buffer
		if len(logBuffer) >= maxLogLines {
			// Rimuovi il primo elemento (shift)
			logBuffer = logBuffer[1:]
		}
		logBuffer = append(logBuffer, msg)

		// Unisci solo le righe necessarie
		finalText := strings.Join(logBuffer, "\n")
		logArea.SetText(finalText)
		logArea.CursorRow = len(logBuffer) - 1
		logArea.Refresh()
	}

	// --- Loop Statistiche Ottimizzato ---
	updateStats := func() {
		nicMutex.RLock()
		defer nicMutex.RUnlock()

		// Ottieni contatori dal sistema
		counters, err := gonet.IOCounters(true)
		if err != nil {
			return
		}
		counterMap := make(map[string]gonet.IOCountersStat)
		for _, c := range counters {
			counterMap[c.Name] = c
		}

		// Aggiorna SOLO i valori dei widget esistenti (Nessuna creazione/distruzione)
		for _, row := range nicRows {
			stat, exists := counterMap[row.Name]
			if !exists {
				continue
			}

			var upRate, downRate float64
			// Calcola delta solo se abbiamo una lettura precedente valida
			if row.PrevSent > 0 {
				elapsed := 1.0 // Approssimazione ticker 1s
				upRate = float64(stat.BytesSent-row.PrevSent) * 8 / 1_000_000 / elapsed
				downRate = float64(stat.BytesRecv-row.PrevRecv) * 8 / 1_000_000 / elapsed
			}
			
			// Aggiorna stato precedente
			row.PrevSent = stat.BytesSent
			row.PrevRecv = stat.BytesRecv

			// Aggiorna UI Text
			row.UpLbl.SetText(fmt.Sprintf("%.2f", upRate))
			row.DownLbl.SetText(fmt.Sprintf("%.2f", downRate))
			row.Graph.AddValue(downRate + upRate)

			// Evidenzia visivamente se attivo
			if row.Check.Checked {
				row.StatsNameLbl.TextStyle = fyne.TextStyle{Bold: true}
				row.StatsNameLbl.SetText(fmt.Sprintf("▶ %s", row.IP))
			} else {
				row.StatsNameLbl.TextStyle = fyne.TextStyle{Bold: false}
				row.StatsNameLbl.SetText(fmt.Sprintf("%s", row.IP))
			}
		}
		// Non serve statsContainer.Refresh() perché aggiorniamo i figli direttamente
	}

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
			statusLabel.SetText("🔴 Proxy: Stopped")
			return
		}

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
			dialog.ShowInformation("Error", "Please select at least one interface", w)
			return
		}

		port, err := strconv.Atoi(portEntry.Text)
		if err != nil {
			dialog.ShowError(fmt.Errorf("invalid port: %v", err), w)
			return
		}

		logger("--- Starting Proxy ---")
		// Avvia in goroutine per non bloccare UI
		go func() {
			err = proxy.Start(hostEntry.Text, port, tunnelCheck.Checked, selected, logger)
			if err != nil {
				logger(fmt.Sprintf("[ERROR] %v", err))
				statusLabel.SetText("🔴 Proxy: Error")
			}
		}()
		
		startBtn.SetText("Stop Proxy")
		startBtn.Importance = widget.HighImportance
		statusLabel.SetText("▶ Proxy: Running")
	}

	w.SetOnClosed(func() {
		close(stopStats)
		if proxy.running {
			proxy.Stop()
		}
	})

	// Init
	refreshNICs()

	// --- Layout Principale ---
	
	// Settings in alto a sinistra
	topSettings := container.NewVBox(
		widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewForm(
			widget.NewFormItem("Host", hostEntry),
			widget.NewFormItem("Port", portEntry),
		),
		tunnelCheck,
		quietCheck,
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabelWithStyle("Interfaces", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			layout.NewSpacer(),
			refreshBtn,
		),
	)

	bottomControls := container.NewVBox(
		widget.NewSeparator(),
		statusLabel,
		startBtn,
	)

	// Lista scrollabile interfacce
	nicScroll := container.NewVScroll(nicContainer)

	leftPanel := container.NewBorder(topSettings, bottomControls, nil, nil, nicScroll)

	rightPanel := container.NewVSplit(
		container.NewBorder(widget.NewLabel("Logs"), nil, nil, nil, logArea),
		container.NewBorder(widget.NewLabel("Real-time Statistics"), nil, nil, nil, container.NewVScroll(statsContainer)),
	)
	rightPanel.SetOffset(0.5) // Log e Grafici al 50/50

	content := container.NewBorder(nil, nil, container.NewPadded(leftPanel), nil, rightPanel)
	w.SetContent(content)
	w.ShowAndRun()
}

type nicInfo struct {
	ip, name string
}

func getValidInterfaces() []nicInfo {
	var res []nicInfo
	ifaces, err := net.Interfaces()
	if err != nil {
		return res
	}

	virtualPatterns := []string{
		"virtual", "vbox", "vmware", "vethernet", "veth",
		"docker", "vpn", "tap", "tun", "host-only",
	}

	for _, i := range ifaces {
		lowerName := strings.ToLower(i.Name)
		isVirtual := false
		for _, pattern := range virtualPatterns {
			if strings.Contains(lowerName, pattern) {
				isVirtual = true
				break
			}
		}
		if isVirtual {
			continue
		}

		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := i.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip string
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP.String()
			case *net.IPAddr:
				ip = v.IP.String()
			}

			if strings.Count(ip, ".") == 3 &&
				!strings.HasPrefix(ip, "127.") &&
				!strings.HasPrefix(ip, "169.254.") &&
				!strings.HasPrefix(ip, "192.168.56.") {
				res = append(res, nicInfo{ip, i.Name})
			}
		}
	}
	return res
}
