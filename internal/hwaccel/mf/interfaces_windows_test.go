package mf

import "testing"

const inheritedFromAttributes = 33

func assertContiguousFrom(t *testing.T, iface string, first int, indices []int) {
	t.Helper()
	if len(indices) == 0 {
		t.Fatalf("%s lists no methods", iface)
	}
	if indices[0] != first {
		t.Fatalf("%s starts at index %d, want %d", iface, indices[0], first)
	}
	for i := 1; i < len(indices); i++ {
		if indices[i] != indices[i-1]+1 {
			t.Fatalf("%s jumps from %d to %d at position %d", iface, indices[i-1], indices[i], i)
		}
	}
}

func TestIUnknownOccupiesTheFirstThreeSlots(t *testing.T) {
	assertContiguousFrom(t, "IUnknown", 0, []int{unknownQueryInterface, unknownAddRef, unknownRelease})
}

func TestTransformMethodsFollowIUnknown(t *testing.T) {
	assertContiguousFrom(t, "IMFTransform", 3, []int{
		transformGetStreamLimits,
		transformGetStreamCount,
		transformGetStreamIDs,
		transformGetInputStreamInfo,
		transformGetOutputStreamInfo,
		transformGetAttributes,
		transformGetInputStreamAttributes,
		transformGetOutputStreamAttributes,
		transformDeleteInputStream,
		transformAddInputStreams,
		transformGetInputAvailableType,
		transformGetOutputAvailableType,
		transformSetInputType,
		transformSetOutputType,
		transformGetInputCurrentType,
		transformGetOutputCurrentType,
		transformGetInputStatus,
		transformGetOutputStatus,
		transformSetOutputBounds,
		transformProcessEvent,
		transformProcessMessage,
		transformProcessInput,
		transformProcessOutput,
	})
}

func TestAttributeMethodsFollowIUnknown(t *testing.T) {
	assertContiguousFrom(t, "IMFAttributes", 3, []int{
		attributesGetItem,
		attributesGetItemType,
		attributesCompareItem,
		attributesCompare,
		attributesGetUINT32,
		attributesGetUINT64,
		attributesGetDouble,
		attributesGetGUID,
		attributesGetStringLength,
		attributesGetString,
		attributesGetAllocatedString,
		attributesGetBlobSize,
		attributesGetBlob,
		attributesGetAllocatedBlob,
		attributesGetUnknown,
		attributesSetItem,
		attributesDeleteItem,
		attributesDeleteAllItems,
		attributesSetUINT32,
		attributesSetUINT64,
		attributesSetDouble,
		attributesSetGUID,
		attributesSetString,
		attributesSetBlob,
		attributesSetUnknown,
		attributesLockStore,
		attributesUnlockStore,
		attributesGetCount,
		attributesGetItemByIndex,
		attributesCopyAllItems,
	})
	if attributesCopyAllItems+1 != inheritedFromAttributes {
		t.Fatalf("IMFAttributes ends at %d, so anything deriving from it must start at %d",
			attributesCopyAllItems, attributesCopyAllItems+1)
	}
}

func TestInterfacesDerivedFromAttributesStartAfterThem(t *testing.T) {
	assertContiguousFrom(t, "IMFMediaType", inheritedFromAttributes, []int{
		mediaTypeGetMajorType,
		mediaTypeIsCompressedFormat,
		mediaTypeIsEqual,
		mediaTypeGetRepresentation,
		mediaTypeFreeRepresentation,
	})
	assertContiguousFrom(t, "IMFSample", inheritedFromAttributes, []int{
		sampleGetSampleFlags,
		sampleSetSampleFlags,
		sampleGetSampleTime,
		sampleSetSampleTime,
		sampleGetSampleDuration,
		sampleSetSampleDuration,
		sampleGetBufferCount,
		sampleGetBufferByIndex,
		sampleConvertToContiguousBuffer,
		sampleAddBuffer,
		sampleRemoveBufferByIndex,
		sampleRemoveAllBuffers,
		sampleGetTotalLength,
		sampleCopyToBuffer,
	})
	assertContiguousFrom(t, "IMFActivate", inheritedFromAttributes, []int{
		activateActivateObject,
		activateShutdownObject,
		activateDetachObject,
	})
	assertContiguousFrom(t, "IMFMediaEvent", inheritedFromAttributes, []int{
		mediaEventGetType,
		mediaEventGetExtendedType,
		mediaEventGetStatus,
		mediaEventGetValue,
	})
}

func TestInterfacesDerivedOnlyFromIUnknownStartAtThree(t *testing.T) {
	assertContiguousFrom(t, "IMFMediaBuffer", 3, []int{
		bufferLock,
		bufferUnlock,
		bufferGetCurrentLength,
		bufferSetCurrentLength,
		bufferGetMaxLength,
	})
	assertContiguousFrom(t, "IMFMediaEventGenerator", 3, []int{
		eventGeneratorGetEvent,
		eventGeneratorBeginGetEvent,
		eventGeneratorEndGetEvent,
		eventGeneratorQueueEvent,
	})
}

func TestTransformMessagesAreDistinct(t *testing.T) {
	seen := map[uint32]bool{}
	for _, m := range []uint32{
		MFTMessageCommandFlush,
		MFTMessageCommandDrain,
		MFTMessageSetD3DManager,
		MFTMessageNotifyBeginStreaming,
		MFTMessageNotifyEndStreaming,
		MFTMessageNotifyEndOfStream,
		MFTMessageNotifyStartOfStream,
	} {
		if seen[m] {
			t.Fatalf("message %#x appears twice", m)
		}
		seen[m] = true
	}
}

func TestTransformErrorsAreFailures(t *testing.T) {
	for _, c := range []HRESULT{
		MFETransformNeedMoreInput,
		MFETransformStreamChange,
		MFEInvalidMediaType,
		MFENotAccepting,
		MFENoMoreTypes,
	} {
		if !c.Failed() {
			t.Fatalf("%s does not read as a failure", c)
		}
	}
}

func TestEnumerationFlagsAreSingleBits(t *testing.T) {
	for _, f := range []uint32{
		MFTEnumFlagSyncMFT,
		MFTEnumFlagAsyncMFT,
		MFTEnumFlagHardware,
		MFTEnumFlagSortAndFilter,
		MFTOutputStatusSampleReady,
		MFTOutputDataBufferIncomplete,
	} {
		if f == 0 || f&(f-1) != 0 {
			t.Fatalf("%#x is not a single bit", f)
		}
	}
}

func TestInterfaceIdentifiersAreDistinct(t *testing.T) {
	all := map[GUID]string{}
	for name, g := range map[string]GUID{
		"IMFTransform":           IIDIMFTransform,
		"IMFMediaType":           IIDIMFMediaType,
		"IMFAttributes":          IIDIMFAttributes,
		"IMFSample":              IIDIMFSample,
		"IMFMediaBuffer":         IIDIMFMediaBuffer,
		"IMFActivate":            IIDIMFActivate,
		"IMFMediaEventGenerator": IIDIMFMediaEventGenerator,
		"IMFMediaEvent":          IIDIMFMediaEvent,
		"ICodecAPI":              IIDICodecAPI,
	} {
		if other, ok := all[g]; ok {
			t.Fatalf("%s and %s carry the same identifier %s", name, other, g)
		}
		all[g] = name
	}
}

func TestMediaAttributeKeysAreDistinct(t *testing.T) {
	all := map[GUID]string{}
	for name, g := range map[string]GUID{
		"MF_MT_MAJOR_TYPE":                MFMTMajorType,
		"MF_MT_SUBTYPE":                   MFMTSubtype,
		"MF_MT_FRAME_SIZE":                MFMTFrameSize,
		"MF_MT_FRAME_RATE":                MFMTFrameRate,
		"MF_MT_AVG_BITRATE":               MFMTAvgBitrate,
		"MF_MT_INTERLACE_MODE":            MFMTInterlaceMode,
		"MF_MT_PIXEL_ASPECT_RATIO":        MFMTPixelAspectRatio,
		"MF_MT_ALL_SAMPLES_INDEPENDENT":   MFMTAllSamplesIndependent,
		"MF_MT_MPEG2_PROFILE":             MFMTMpeg2Profile,
		"MF_MT_MPEG2_LEVEL":               MFMTMpeg2Level,
		"MF_MT_DEFAULT_STRIDE":            MFMTDefaultStride,
		"MF_MT_MPEG_SEQUENCE_HEADER":      MFMTMpegSequenceHeader,
		"MF_TRANSFORM_ASYNC":              MFTransformAsync,
		"MF_TRANSFORM_ASYNC_UNLOCK":       MFTransformAsyncUnlock,
		"MFT_FRIENDLY_NAME_Attribute":     MFTFriendlyNameAttribute,
		"MFT_ENUM_HARDWARE_URL_Attribute": MFTEnumHardwareURLAttribute,
		"MF_SA_D3D11_AWARE":               MFSAD3D11Aware,
		"MF_LOW_LATENCY":                  MFLowLatency,
		"MFT_CATEGORY_VIDEO_ENCODER":      MFTCategoryVideoEncoder,
		"MFT_CATEGORY_VIDEO_DECODER":      MFTCategoryVideoDecoder,
	} {
		if other, ok := all[g]; ok {
			t.Fatalf("%s and %s carry the same identifier %s", name, other, g)
		}
		all[g] = name
	}
}

func TestH264ProfilesCarryTheirSpecificationNumbers(t *testing.T) {
	if AVEncH264VProfileBase != 66 || AVEncH264VProfileMain != 77 || AVEncH264VProfileHigh != 100 {
		t.Fatalf("the profile values are %d, %d and %d, want 66, 77 and 100",
			AVEncH264VProfileBase, AVEncH264VProfileMain, AVEncH264VProfileHigh)
	}
	if MFVideoInterlaceProgressive != 2 {
		t.Fatalf("progressive interlace mode is %d, want 2", MFVideoInterlaceProgressive)
	}
}
