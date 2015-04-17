/*
Copyright 2015 Google Inc. All rights reserved.

Use of this source code is governed by a BSD-style
license that can be found in the LICENSE file or at
https://developers.google.com/open-source/licenses/bsd
*/
package lib

import (
	"crypto/md5"
	"fmt"
	"sort"
	"strings"
	"time"
)

type PrinterState uint8

// CUPS: ipp_pstate_t; GCP: CloudDeviceState.StateType
const (
	PrinterIdle       PrinterState = iota // CUPS: IPP_PSTATE_IDLE;       GCP: StateType.IDLE
	PrinterProcessing              = iota // CUPS: IPP_PSTATE_PROCESSING; GCP: StateType.PROCESSING
	PrinterStopped                 = iota // CUPS: IPP_PSTATE_STOPPED;    GCP: StateType.STOPPED
)

func PrinterStateFromCUPS(ss string) PrinterState {
	switch strings.ToLower(ss) {
	case "3":
		return PrinterIdle
	case "4":
		return PrinterProcessing
	case "5":
		return PrinterStopped
	default:
		return PrinterIdle
	}
}

func (ps PrinterState) GCPPrinterState() string {
	switch ps {
	case PrinterIdle:
		return "IDLE"
	case PrinterProcessing:
		return "PROCESSING"
	case PrinterStopped:
		return "STOPPED"
	default:
		return "IDLE"
	}
}

func PrinterStateFromGCP(ss string) PrinterState {
	switch strings.ToLower(ss) {
	case "IDLE":
		return PrinterIdle
	case "PROCESSING":
		return PrinterProcessing
	case "STOPPED":
		return PrinterStopped
	default:
		return PrinterIdle
	}
}

// CUPS: cups_dest_t; GCP: /register and /update interfaces
type Printer struct {
	GCPID              string            //                                    GCP: printerid (GCP key)
	Name               string            // CUPS: cups_dest_t.name (CUPS key); GCP: name field
	DefaultDisplayName string            // CUPS: printer-info;                GCP: default_display_name field
	UUID               string            // CUPS: printer-uuid;                GCP: uuid field
	State              PrinterState      // CUPS: printer-state;               GCP: CDS.StateType
	StateReasons       []string          // CUPS: printer-state-reasons;       GCP: CDS.PrinterStateSection fields
	CapsHash           string            // CUPS: hash of PPD;                 GCP: capsHash field
	Tags               map[string]string // CUPS: all printer attributes;      GCP: repeated tag field
	XMPPPingInterval   time.Duration     //                                    GCP: local_settings/xmpp_timeout_value field
	CUPSJobSemaphore   *Semaphore
}

// SetTagshash calculates an MD5 sum for the Printer.Tags map,
// sets Printer.Tags["tagshash"] to that value.
func (p *Printer) SetTagshash() {
	sortedKeys := make([]string, len(p.Tags))
	i := 0
	for key := range p.Tags {
		sortedKeys[i] = key
		i++
	}
	sort.Strings(sortedKeys)

	tagshash := md5.New()
	for _, key := range sortedKeys {
		if key == "tagshash" {
			continue
		}
		tagshash.Write([]byte(key))
		tagshash.Write([]byte(p.Tags[key]))
	}

	p.Tags["tagshash"] = fmt.Sprintf("%x", tagshash.Sum(nil))
}

type PrinterDiffOperation int8

const (
	RegisterPrinter PrinterDiffOperation = iota
	UpdatePrinter
	DeletePrinter
	NoChangeToPrinter
)

// Describes changes to be pushed to a GCP printer.
type PrinterDiff struct {
	Operation PrinterDiffOperation
	Printer   Printer

	DefaultDisplayNameChanged bool
	UUIDChanged               bool
	StateChanged              bool // Also indicates changes to StateReasons.
	CapsHashChanged           bool
	XMPPPingIntervalChanged   bool
	TagsChanged               bool
}

func printerSliceToMapByName(s []Printer) map[string]Printer {
	m := make(map[string]Printer, len(s))
	for i := range s {
		m[s[i].Name] = s[i]
	}
	return m
}

// DiffPrinters returns the diff between old (GCP) and new (CUPS) printers.
// Returns nil if zero printers or if all diffs are NoChangeToPrinter operation.
func DiffPrinters(cupsPrinters, gcpPrinters []Printer) []PrinterDiff {
	// So far, no changes.
	dirty := false

	diffs := make([]PrinterDiff, 0, 1)
	printersConsidered := make(map[string]bool, len(cupsPrinters))
	cupsPrintersByName := printerSliceToMapByName(cupsPrinters)

	for i := range gcpPrinters {
		if printersConsidered[gcpPrinters[i].Name] {
			// GCP can have multiple printers with one name. Remove dupes.
			diffs = append(diffs, PrinterDiff{Operation: DeletePrinter, Printer: gcpPrinters[i]})
			dirty = true

		} else {
			printersConsidered[gcpPrinters[i].Name] = true

			if cupsPrinter, exists := cupsPrintersByName[gcpPrinters[i].Name]; exists {
				// CUPS printer doesn't know about GCPID yet.
				cupsPrinter.GCPID = gcpPrinters[i].GCPID
				// CUPS printer doesn't know about XMPP ping interval yet.
				cupsPrinter.XMPPPingInterval = gcpPrinters[i].XMPPPingInterval
				// Don't lose track of this semaphore.
				cupsPrinter.CUPSJobSemaphore = gcpPrinters[i].CUPSJobSemaphore

				diff := diffPrinter(&cupsPrinter, &gcpPrinters[i])
				diffs = append(diffs, diff)

				if diff.Operation != NoChangeToPrinter {
					dirty = true
				}

			} else {
				diffs = append(diffs, PrinterDiff{Operation: DeletePrinter, Printer: gcpPrinters[i]})
				dirty = true
			}
		}
	}

	for i := range cupsPrinters {
		if !printersConsidered[cupsPrinters[i].Name] {
			diffs = append(diffs, PrinterDiff{Operation: RegisterPrinter, Printer: cupsPrinters[i]})
			dirty = true
		}
	}

	if dirty {
		return diffs
	} else {
		return nil
	}
}

// diffPrinter finds the difference between a CUPS printer and the corresponding GCP printer.
//
// pc: printer-CUPS; the thing that is correct
//
// pg: printer-GCP; the thing that will be updated
func diffPrinter(pc, pg *Printer) PrinterDiff {
	d := PrinterDiff{
		Operation: UpdatePrinter,
		Printer:   *pc,
	}

	if pg.DefaultDisplayName != pc.DefaultDisplayName {
		d.DefaultDisplayNameChanged = true
	}
	if pg.UUID != pc.UUID {
		d.UUIDChanged = true
	}
	if pg.State != pc.State {
		d.StateChanged = true
	} else if len(pg.StateReasons) != len(pc.StateReasons) {
		d.StateChanged = true
	} else {
		for i := range pg.StateReasons {
			if pg.StateReasons[i] != pc.StateReasons[i] {
				d.StateChanged = true
				break
			}
		}
	}
	if pg.CapsHash != pc.CapsHash {
		d.CapsHashChanged = true
	}
	if pg.XMPPPingInterval != pc.XMPPPingInterval {
		d.XMPPPingIntervalChanged = true
	}

	gcpTagshash, gcpHasTagshash := pg.Tags["tagshash"]
	cupsTagshash, cupsHasTagshash := pc.Tags["tagshash"]
	if !gcpHasTagshash || !cupsHasTagshash || gcpTagshash != cupsTagshash {
		d.TagsChanged = true
	}

	if d.DefaultDisplayNameChanged || d.UUIDChanged || d.StateChanged ||
		d.CapsHashChanged || d.XMPPPingIntervalChanged || d.TagsChanged {
		return d
	}

	return PrinterDiff{
		Operation: NoChangeToPrinter,
		Printer:   *pg,
	}
}

// FilterRawPrinters splits a slice of printers into non-raw and raw.
func FilterRawPrinters(printers []Printer) ([]Printer, []Printer) {
	notRaw, raw := make([]Printer, 0, len(printers)), make([]Printer, 0, 0)
	for i := range printers {
		if PrinterIsRaw(printers[i]) {
			raw = append(raw, printers[i])
		} else {
			notRaw = append(notRaw, printers[i])
		}
	}
	return notRaw, raw
}

func PrinterIsRaw(printer Printer) bool {
	if printer.Tags["printer-make-and-model"] == "Local Raw Printer" {
		return true
	}
	return false
}
