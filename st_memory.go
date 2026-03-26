package m68kemu

const (
	STCartridgeStart uint32 = 0xFA0000
	STCartridgeEnd   uint32 = 0xFBFFFF
	STTOSStart       uint32 = 0xFC0000
	STTOSEnd         uint32 = 0xFEFFFF
	STIOStart        uint32 = 0xFF8000
	STIOEnd          uint32 = 0xFFFFFF
)

type STRegionMapping struct {
	Start  uint32
	End    uint32
	Device Device
}

func NewAtariSTBus(mappings ...STRegionMapping) *Bus {
	devices := make([]Device, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.Device == nil {
			continue
		}
		devices = append(devices, MapDevice(mapping.Start, mapping.End, mapping.Device))
	}
	return NewBus(devices...)
}
