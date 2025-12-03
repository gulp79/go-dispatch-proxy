package main

import (
	"math/big"
	"sync"
)

// Backend rappresenta un singolo server o interfaccia di uscita
type Backend struct {
	Address            string
	Interface          string
	ContentionRatio    int
	CurrentConnections int
}

// Dispatcher gestisce il pool di backend e la logica di selezione
type Dispatcher struct {
	backends []*Backend
	mu       sync.Mutex
	index    int
}

// NewDispatcher crea una nuova istanza
func NewDispatcher(backends []*Backend) *Dispatcher {
	return &Dispatcher{
		backends: backends,
		index:    0,
	}
}

// Next restituisce il prossimo backend da utilizzare (Round Robin con Contention Ratio)
// Restituisce anche l'indice per loggare quale LB è stato usato.
func (d *Dispatcher) Next(retryParams ...interface{}) (*Backend, int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Gestione logica retry (se implementata come bitset, qui semplificata per chiarezza)
	// Se vengono passati parametri di retry, si potrebbe forzare un salto, 
	// ma per mantenere il codice pulito usiamo la logica standard qui.
	
	if len(d.backends) == 0 {
		return nil, -1
	}

	lb := d.backends[d.index]
	currentIndex := d.index

	lb.CurrentConnections++

	// Se abbiamo raggiunto il ratio, passiamo al prossimo
	if lb.CurrentConnections >= lb.ContentionRatio {
		lb.CurrentConnections = 0
		d.index++
		if d.index >= len(d.backends) {
			d.index = 0
		}
	}

	return lb, currentIndex
}

// GetBackendsCount restituisce il numero di backend configurati
func (d *Dispatcher) Count() int {
	return len(d.backends)
}

// Bitset logic helper per i retry (adattato dall'originale)
func (d *Dispatcher) GetNextFailed(failedIndices *big.Int) (*Backend, int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := 0; i < len(d.backends); i++ {
		// Cerca un indice non ancora fallito
		idx := (d.index + i) % len(d.backends)
		if failedIndices.Bit(idx) == 0 {
			return d.backends[idx], idx
		}
	}
	return nil, -1
}