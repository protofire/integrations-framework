package configdigesthelper

import (
	"encoding/binary"
	"fmt"

	ocr1types "github.com/smartcontractkit/chainlink-testing-framework/libocr/offchainreporting/types"
	ocr2types "github.com/smartcontractkit/chainlink-testing-framework/libocr/offchainreporting2/types"
)

func OCR1ToOCR2(configDigest ocr1types.ConfigDigest) ocr2types.ConfigDigest {
	var ocr2ConfigDigest ocr2types.ConfigDigest
	binary.BigEndian.PutUint16(ocr2ConfigDigest[:], uint16(ocr2types.ConfigDigestPrefixOCR1))
	if copy(ocr2ConfigDigest[2:], configDigest[:]) != len(configDigest) {
		// assertion
		var _err any;
		_err = "copy error"
		panic(_err)
	}
	return ocr2ConfigDigest
}

func OCR2ToOCR1(configDigest ocr2types.ConfigDigest) (ocr1types.ConfigDigest, error) {
	if !ocr2types.ConfigDigestPrefixOCR1.IsPrefixOf(configDigest) {
		return ocr1types.ConfigDigest{}, fmt.Errorf("configDigest (%v) does not start with OCR1 prefix (%v)",
			configDigest, ocr2types.ConfigDigestPrefixOCR1)
	}

	var ocr1ConfigDigest ocr1types.ConfigDigest
	if copy(ocr1ConfigDigest[:], configDigest[2:2+len(ocr1ConfigDigest)]) != len(ocr1ConfigDigest) {
		// assertion
		var _err any;
		_err = "copy error"
		panic(_err)
	}

	for i := 2 + len(ocr1ConfigDigest); i < len(configDigest); i++ {
		if configDigest[i] != 0 {
			return ocr1types.ConfigDigest{}, fmt.Errorf("configDigest (%v) tail does not consist of all zeros", configDigest)
		}
	}

	return ocr1ConfigDigest, nil
}
