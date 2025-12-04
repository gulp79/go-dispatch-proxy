package main

import (
	"fmt"
	"image/color"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/shirou/gopsutil/v3/net"
)

var proxy ProxyServer

// Struttura per tracciare lo stato delle NIC nella GUI
type NICRow struct {
	Name      string
	IP        string
	Check     *widget.Check
	Slider    *widget.Slider
	ValueLbl  *widget.Label
	
	// Stats widgets
	UpLbl     *widget.Label
	DownLbl   *widget.Label
	Graph     *MiniGraph
	PrevSent  uint64
	PrevRecv  uint64
}

func main() {
	a := app.NewWithID("com.gulp79.dispatchproxy")
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
	nicRows := make(map[string]*NICRow) // Key: IP address

	refreshNICs := func() {
		nicContainer.Objects = nil
		loadedNICs := getValidInterfaces()
		
		// Ordina per IP
		sort.Slice(loadedNICs, func(i, j int) bool { return loadedNICs[i].ip < loadedNICs[j].ip })

		for _, nic := range loadedNICs {
			// Row UI
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

			// Stats Elements
			upL := widget.NewLabel("0.0")
			downL := widget.NewLabel("0.0")
			gr := NewMiniGraph(theme.PrimaryColor())
			
			row := &NICRow{
				Name: nic.name, IP: nic.ip, Check: chk, Slider: sl, ValueLbl: valLbl,
				UpLbl: upL, DownLbl: downL, Graph: gr,
			}
			nicRows[nic.ip] = row

			// Layout riga selezione (Checkbox | Label | Slider | Value)
			topRow := container.NewBorder(nil, nil, chk, container.NewHBox(valLbl, sl), lbl)
			nicContainer.Add(topRow)
		}
		nicContainer.Refresh()
	}

	refreshBtn := widget.NewButton("Refresh Interfaces", refreshNICs)
	
	startBtn := widget.NewButton("Start Proxy", nil) // Definito dopo

	// --- Right Panel Components ---
	logArea := widget.NewMultiLineEntry()
	logArea.TextStyle = fyne.TextStyle{Monospace: true}
	logArea.Wrapping = fyne.TextWrapBreak
	logArea.Disable() // Read only
	
	logger := func(msg string) {
		if quietCheck.Checked && strings.Contains(msg, "[DEBUG]") { return }
		logArea.SetText(logArea.Text + msg + "\n")
		logArea.CursorRow = len(strings.Split(logArea.Text, "\n"))
		logArea.Refresh()
	}

	// --- Stats Grid ---
	statsContainer := container.NewVBox()
	
	updateStats := func() {
		statsContainer.Objects = nil
		// Header
		statsContainer.Add(container.NewGridWithColumns(4, 
			widget.NewLabelWithStyle("Interface", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Upload (Mb/s)", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Download (Mb/s)", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Activity", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		))
		
		counters, _ := net.IOCounters(true)
		counterMap := make(map[string]net.IOCountersStat)
		for _, c := range counters { counterMap[c.Name] = c }

		// Ordina le chiavi
		ips := make([]string, 0, len(nicRows))
		for ip := range nicRows { ips = append(ips, ip) }
		sort.Strings(ips)

		for _, ip := range ips {
			row := nicRows[ip]
			stat, exists := counterMap[row.Name]
			if !exists { continue }

			// Calcolo delta
			now := time.Now() // Approssimazione: assumiamo chiamata ogni 1s
			_ = now
			
			// Primo giro ignora
			var upRate, downRate float64
			if row.PrevSent > 0 {
				upRate = float64(stat.BytesSent - row.PrevSent) * 8 / 1_000_000
				downRate = float64(stat.BytesRecv - row.PrevRecv) * 8 / 1_000_000
			}
			row.PrevSent = stat.BytesSent
			row.PrevRecv = stat.BytesRecv
			
			row.UpLbl.SetText(fmt.Sprintf("%.2f", upRate))
			row.DownLbl.SetText(fmt.Sprintf("%.2f", downRate))
			row.Graph.AddValue(downRate) // Grafichiamo il download

			// Riga Statistica
			statsContainer.Add(container.NewGridWithColumns(4,
				widget.NewLabel(fmt.Sprintf("%s (%s)", row.IP, row.Name)),
				row.UpLbl,
				row.DownLbl,
				container.NewPadded(row.Graph),
			))
		}
		statsContainer.Refresh()
	}

	// Loop aggiornamento stats
	go func() {
		t := time.NewTicker(1 * time.Second)
		for range t.C {
			updateStats()
		}
	}()

	// Start Logic
	startBtn.OnTapped = func() {
		if proxy.running {
			proxy.Stop()
			startBtn.SetText("Start Proxy")
			startBtn.Importance = widget.MediumImportance
			return
		}

		// Raccogli configurazione
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

		if len(selected) == 0 {
			dialog := widget.NewModalPopUp(widget.NewLabel("Please select at least one interface"), w.Canvas())
			dialog.Show()
			return
		}

		port, _ := strconv.Atoi(portEntry.Text)
		logger("--- Starting Proxy ---")
		err := proxy.Start(hostEntry.Text, port, tunnelCheck.Checked, selected, logger)
		if err != nil {
			logger(fmt.Sprintf("[ERROR] %v", err))
		} else {
			startBtn.SetText("Stop Proxy")
			startBtn.Importance = widget.HighImportance // Diventa rosso/evidenziato
		}
	}

	// Init
	refreshNICs()

	// Layout Finale
	leftPanel := container.NewVBox(
		widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Size: 18}),
		widget.NewForm(
			widget.NewFormItem("Host", hostEntry),
			widget.NewFormItem("Port", portEntry),
		),
		tunnelCheck,
		quietCheck,
		layout.NewSpacer(),
		widget.NewLabelWithStyle("Interfaces", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		refreshBtn,
		container.NewVScroll(nicContainer), // Scrollabile se tante interfacce
		layout.NewSpacer(),
		startBtn,
	)
	
	// Right Panel
	rightPanel := container.NewVSplit(
		container.NewBorder(widget.NewLabel("Logs"), nil, nil, nil, logArea),
		container.NewBorder(widget.NewLabel("Real-time Statistics"), nil, nil, nil, container.NewVScroll(statsContainer)),
	)
	rightPanel.SetOffset(0.6) // 60% logs, 40% stats

	content := container.NewBorder(nil, nil, container.NewPadded(leftPanel), nil, rightPanel)
	w.SetContent(content)
	w.ShowAndRun()
}

type nicInfo struct { ip, name string }
func getValidInterfaces() []nicInfo {
	var res []nicInfo
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		// Filtra loopback o down (basic logic, gopsutil aiuta)
		if strings.Contains(strings.ToLower(i.Name), "loop") { continue }
		
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			ip := addr.Addr
			// Semplice check IPv4 non-local
			if strings.Count(ip, ".") == 3 && !strings.HasPrefix(ip, "127.") {
				res = append(res, nicInfo{ip, i.Name})
			}
		}
	}
	return res

}
