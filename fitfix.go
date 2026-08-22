package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
)

// FIT protocol invalid sentinel values per base type.
const (
	uint8Invalid  = uint8(0xFF)
	uint16Invalid = uint16(0xFFFF)
	sint8Invalid  = int8(0x7F)
)

const (
	garminManufacturer = typedef.ManufacturerGarmin // 1
	fitSerialNumber    = uint32(3420897194)
)

// garminGear identifies the Garmin FIT product encoded in uploaded activities.
type garminGear struct {
	product typedef.GarminProduct
	name    string
}

// parseGarminGear accepts common model names or a numeric Garmin FIT product ID.
func parseGarminGear(value string) (garminGear, error) {
	raw := strings.TrimSpace(value)
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(raw))

	switch normalized {
	case "forerunner265", "fr265":
		return garminGear{typedef.GarminProductFr265Large, "Garmin Forerunner 265"}, nil
	case "forerunner265s", "fr265s":
		return garminGear{typedef.GarminProductFr265Small, "Garmin Forerunner 265S"}, nil
	}

	productID, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || productID == 0 {
		return garminGear{}, fmt.Errorf("invalid GARMIN_GEAR %q; use forerunner-265, forerunner-265s, or a Garmin FIT product ID", value)
	}
	return garminGear{
		product: typedef.GarminProduct(productID),
		name:    fmt.Sprintf("Garmin product %d", productID),
	}, nil
}

// logFn can be overridden to redirect log output (e.g., in tests).
var logFn = func(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// ---------------------------------------------------------------------------
// FIT file processing
// ---------------------------------------------------------------------------

// fixFitFile reads a MyWhoosh FIT activity, fixes missing session averages,
// strips temperature from records, sets the selected Garmin gear, and writes the result.
func fixFitFile(inputPath, outputPath string, gear garminGear) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	lis := filedef.NewListener()
	defer lis.Close()

	dec := decoder.New(f,
		decoder.WithMesgListener(lis),
		decoder.WithBroadcastOnly(),
	)

	_, err = dec.Decode()
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	activity, ok := lis.File().(*filedef.Activity)
	if !ok {
		return fmt.Errorf("not an activity file (got %T)", lis.File())
	}

	// Collect metrics from records and strip temperature
	var powers []uint16
	var heartRates, cadences []uint8

	for _, rec := range activity.Records {
		if rec.Power != uint16Invalid {
			powers = append(powers, rec.Power)
		}
		if rec.HeartRate != uint8Invalid {
			heartRates = append(heartRates, rec.HeartRate)
		}
		if rec.Cadence != uint8Invalid {
			cadences = append(cadences, rec.Cadence)
		}
		rec.Temperature = sint8Invalid
	}

	logFn("Records: %d | Power: %d | HR: %d | Cadence: %d samples\n",
		len(activity.Records), len(powers), len(heartRates), len(cadences))

	// Fix missing session averages
	for _, sess := range activity.Sessions {
		if shouldFixU16(sess.AvgPower) && len(powers) > 0 {
			sess.AvgPower = avgU16(powers)
			logFn("  → avg power:      %d W\n", sess.AvgPower)
		}
		if shouldFixU8(sess.AvgHeartRate) && len(heartRates) > 0 {
			sess.AvgHeartRate = avgU8(heartRates)
			logFn("  → avg heart rate: %d bpm\n", sess.AvgHeartRate)
		}
		if shouldFixU8(sess.AvgCadence) && len(cadences) > 0 {
			sess.AvgCadence = avgU8(cadences)
			logFn("  → avg cadence:    %d rpm\n", sess.AvgCadence)
		}
	}

	// Set the Garmin gear selected through GARMIN_GEAR.
	setGarminGear(activity, gear)

	// Encode
	fit := activity.ToFIT(nil)

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	return encoder.New(out, encoder.WithProtocolVersion(proto.V2)).Encode(&fit)
}

func shouldFixU16(v uint16) bool { return v == uint16Invalid || v == 0 }
func shouldFixU8(v uint8) bool   { return v == uint8Invalid || v == 0 }

func avgU16(vals []uint16) uint16 {
	var sum uint64
	for _, v := range vals {
		sum += uint64(v)
	}
	return uint16(sum / uint64(len(vals)))
}

func avgU8(vals []uint8) uint8 {
	var sum uint64
	for _, v := range vals {
		sum += uint64(v)
	}
	return uint8(sum / uint64(len(vals)))
}

// ---------------------------------------------------------------------------
// Garmin gear
// ---------------------------------------------------------------------------

func setGarminGear(activity *filedef.Activity, gear garminGear) {
	activity.FileId.Manufacturer = garminManufacturer
	activity.FileId.Product = gear.product.Uint16()
	activity.FileId.ProductName = gear.name
	activity.FileId.SerialNumber = fitSerialNumber

	for _, di := range activity.DeviceInfos {
		di.Manufacturer = garminManufacturer
		di.Product = gear.product.Uint16()
		di.ProductName = gear.name
		di.SerialNumber = fitSerialNumber
	}

	logFn("  → Garmin gear: %s (product %d)\n", gear.name, gear.product)
}
