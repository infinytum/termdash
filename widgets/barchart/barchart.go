// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package barchart implements a widget that draws multiple bars displaying
// values and their relative ratios.
package barchart

import (
	"errors"
	"fmt"
	"image"
	"sync"

	"github.com/mum4k/termdash/align"
	"github.com/mum4k/termdash/canvas"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/draw"
	"github.com/mum4k/termdash/terminalapi"
	"github.com/mum4k/termdash/widgetapi"
)

// BarChart displays multiple bars showing relative ratios of values.
//
// Each bar can have a text label under it explaining the meaning of the value
// and can display the value itself inside the bar.
//
// Implements widgetapi.Widget. This object is thread-safe.
type BarChart struct {
	// values are the values provided on a call to Values(). These are the
	// individual bars that will be drawn.
	values []int
	// max is the maximum value of a bar. A bar having this value takes all the
	// vertical space.
	max int

	// mu protects the BarChart.
	mu sync.Mutex

	// opts are the provided options.
	opts *options
}

// New returns a new BarChart.
func New(opts ...Option) *BarChart {
	opt := newOptions()
	for _, o := range opts {
		o.set(opt)
	}
	return &BarChart{
		opts: opt,
	}
}

// Draw draws the BarChart widget onto the canvas.
// Implements widgetapi.Widget.Draw.
func (bc *BarChart) Draw(cvs *canvas.Canvas) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	for i, v := range bc.values {
		r, err := bc.barRect(cvs, i, v)
		if err != nil {
			return err
		}

		if r.Dy() > 0 { // Value might be so small so that the rectangle is zero.
			if err := draw.Rectangle(cvs, r,
				draw.RectCellOpts(cell.BgColor(bc.barColor(i))),
				draw.RectChar(bc.opts.barChar),
			); err != nil {
				return err
			}
		}

		if bc.opts.showValues {
			if err := bc.drawText(cvs, i, fmt.Sprint(bc.values[i]), bc.valColor(i), insideBar); err != nil {
				return err
			}
		}

		l, c := bc.label(i)
		if l != "" {
			if err := bc.drawText(cvs, i, l, c, underBar); err != nil {
				return err
			}
		}
	}
	return nil
}

// textLoc represents the location of the drawn text.
type textLoc int

const (
	insideBar textLoc = iota
	underBar
)

// drawText draws the provided text inside or under the i-th bar.
func (bc *BarChart) drawText(cvs *canvas.Canvas, i int, text string, color cell.Color, loc textLoc) error {
	// Rectangle representing area in which the text will be aligned.
	var barCol image.Rectangle

	r, err := bc.barRect(cvs, i, bc.max)
	if err != nil {
		return err
	}

	switch loc {
	case insideBar:
		// Align the text within the bar itself.
		barCol = r
	case underBar:
		// Align the text within the entire column where the bar is, this
		// includes the space for any label under the bar.
		barCol = image.Rect(r.Min.X, cvs.Area().Min.Y, r.Max.X, cvs.Area().Max.Y)
	}

	start, err := align.Text(barCol, text, align.HorizontalCenter, align.VerticalBottom)
	if err != nil {
		return err
	}

	return draw.Text(cvs, text, start,
		draw.TextCellOpts(cell.FgColor(color)),
		draw.TextMaxX(barCol.Max.X),
		draw.TextOverrunMode(draw.OverrunModeThreeDot),
	)
}

// barWidth determines the width of a single bar based on options and the canvas.
func (bc *BarChart) barWidth(cvs *canvas.Canvas) int {
	if len(bc.values) == 0 {
		return 0 // No width when we have no values.
	}

	if bc.opts.barWidth >= 1 {
		// Prefer width set via the options if it is positive.
		return bc.opts.barWidth
	}

	gaps := len(bc.values) - 1
	gapW := gaps * bc.opts.barGap
	rem := cvs.Area().Dx() - gapW
	return rem / len(bc.values)
}

// barHeight determines the height of the i-th bar based on the value it is displaying.
func (bc *BarChart) barHeight(cvs *canvas.Canvas, i, value int) int {
	available := cvs.Area().Dy()
	if len(bc.opts.labels) > 0 {
		// One line for the bar labels.
		available--
	}

	ratio := float32(value) / float32(bc.max)
	return int(float32(available) * ratio)
}

// barRect returns a rectangle that represents the i-th bar on the canvas that
// displays the specified value.
func (bc *BarChart) barRect(cvs *canvas.Canvas, i, value int) (image.Rectangle, error) {
	bw := bc.barWidth(cvs)
	minX := bw * i
	if i > 0 {
		minX += bc.opts.barGap * i
	}
	maxX := minX + bw

	bh := bc.barHeight(cvs, i, value)
	maxY := cvs.Area().Max.Y
	if len(bc.opts.labels) > 0 {
		// One line for the bar labels.
		maxY--
	}
	minY := maxY - bh
	return image.Rect(minX, minY, maxX, maxY), nil
}

// barColor safely determines the color for the i-th bar.
// Colors are optional and don't have to be specified for all the bars.
func (bc *BarChart) barColor(i int) cell.Color {
	if len(bc.opts.barColors) > i {
		return bc.opts.barColors[i]
	}
	return DefaultBarColor
}

// valColor safely determines the color for the i-th value.
// Colors are optional and don't have to be specified for all the values.
func (bc *BarChart) valColor(i int) cell.Color {
	if len(bc.opts.valueColors) > i {
		return bc.opts.valueColors[i]
	}
	return DefaultValueColor
}

// label safely determines the label and its color for the i-th bar.
// Labels are optional and don't have to be specified for all the bars.
func (bc *BarChart) label(i int) (string, cell.Color) {
	var label string
	if len(bc.opts.labels) > i {
		label = bc.opts.labels[i]
	}

	if len(bc.opts.labelColors) > i {
		return label, bc.opts.labelColors[i]
	}
	return label, DefaultLabelColor
}

// Values sets the values to be displayed by the BarChart.
// Each value ends up in its own bar. The values must not be negative and must
// be less or equal the maximum value. A bar displaying the maximum value is a
// full bar, taking all available vertical space.
// Provided options override values set when New() was called.
func (bc *BarChart) Values(values []int, max int, opts ...Option) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if err := validateValues(values, max); err != nil {
		return err
	}

	for _, opt := range opts {
		opt.set(bc.opts)
	}
	bc.values = values
	bc.max = max
	return nil
}

// Keyboard input isn't supported on the BarChart widget.
func (*BarChart) Keyboard(k *terminalapi.Keyboard) error {
	return errors.New("the BarChart widget doesn't support keyboard events")
}

// Mouse input isn't supported on the BarChart widget.
func (*BarChart) Mouse(m *terminalapi.Mouse) error {
	return errors.New("the BarChart widget doesn't support mouse events")
}

// Options implements widgetapi.Widget.Options.
func (bc *BarChart) Options() widgetapi.Options {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return widgetapi.Options{
		MinimumSize:  bc.minSize(),
		WantKeyboard: false,
		WantMouse:    false,
	}
}

// minSize determines the minimum required size of the canvas.
func (bc *BarChart) minSize() image.Point {
	bars := len(bc.values)
	if bars == 0 {
		return image.Point{1, 1}
	}

	minHeight := 1 // At least one character vertically to display the bar.
	if len(bc.opts.labels) > 0 {
		minHeight++ // One line for the labels.
	}

	var minBarWidth int
	if bc.opts.barWidth < 1 {
		minBarWidth = 1 // At least one char for the bar itself.
	} else {
		minBarWidth = bc.opts.barWidth
	}
	minWidth := bars*minBarWidth + (bars-1)*bc.opts.barGap
	return image.Point{minWidth, minHeight}
}

// validateValues validates the provided values and maximum.
func validateValues(values []int, max int) error {
	if max < 1 {
		return fmt.Errorf("invalid maximum value %d, must be at least 1", max)
	}

	for i, v := range values {
		if v < 0 || v > max {
			return fmt.Errorf("invalid values[%d]: %d, each value must be 0 <= value <= max", i, v)
		}
	}
	return nil
}
