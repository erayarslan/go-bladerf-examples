package fm_radio

import (
	"github.com/erayarslan/go-bladerf"
	"github.com/gordonklaus/portaudio"
	"github.com/racerxdl/segdsp/demodcore"
	"os"
)

const bufferSize = 2048

const sourceFrequency = 96600000
const sourceSampleRate = 4000000
const sourceBandwidth = 240000
const sourceNumberOfBuffer = 16
const sourceNumberOfTransfer = 8

const demodulatorSampleRate = 2000000
const demodulatorSignalBw = 80000
const demodulatorOutputRate = 48000

const outputSampleRate = 48000
const outputBufferSize = bufferSize * 8

var audioChannel = make(chan []float32)
var demodulator = demodcore.MakeWBFMDemodulator(demodulatorSampleRate, demodulatorSignalBw, demodulatorOutputRate)

func Int16ToComplex64(data []int16) []complex64 {
	var complexFloat = make([]complex64, len(data)/2)

	for i := 0; i < len(complexFloat); i++ {
		complexFloat[i] = complex(float32(data[2*i])/2048, float32(data[2*i+1])/2048)
	}

	return complexFloat
}

func ConfigureSDR() bladerf.BladeRF {
	rf, err := bladerf.Open()

	if err != nil {
		panic(err)
	}

	channel := bladerf.ChannelRx(0)
	_ = rf.SetFrequency(channel, sourceFrequency)
	_, _ = rf.SetSampleRate(channel, sourceSampleRate)
	_, _ = rf.SetBandwidth(channel, sourceBandwidth)
	_ = rf.EnableModule(channel)

	return rf
}

func DataToAudioChannel(data []int16) {
	if out := demodulator.Work(Int16ToComplex64(data)); out != nil {
		audioChannel <- out.(demodcore.DemodData).Data
	}
}

func AsyncCallback(data []int16) bladerf.GoStream {
	DataToAudioChannel(data)
	return bladerf.GoStreamNext
}

func ConfigureAudioStream() {
	_ = portaudio.Initialize()
	hostApi, _ := portaudio.DefaultHostApi()

	parameters := portaudio.LowLatencyParameters(nil, hostApi.DefaultOutputDevice)
	parameters.Input.Channels = 0
	parameters.Output.Channels = 1
	parameters.SampleRate = outputSampleRate
	parameters.FramesPerBuffer = outputBufferSize

	audioStream, _ := portaudio.OpenStream(parameters, func(out []float32) {
		copy(out, <-audioChannel)
	})

	_ = audioStream.Start()
}

func Async(rf bladerf.BladeRF) {
	rxStream, _ := rf.InitStream(bladerf.FormatSc16Q11, sourceNumberOfBuffer, bufferSize, sourceNumberOfTransfer, AsyncCallback)
	defer rxStream.DeInit()
	ConfigureAudioStream()
	_ = rxStream.Start(bladerf.RxX1)
}

func Sync(rf bladerf.BladeRF) {
	_ = rf.SyncConfig(bladerf.RxX1, bladerf.FormatSc16Q11, sourceNumberOfBuffer, bufferSize, sourceNumberOfTransfer, 32)
	ConfigureAudioStream()
	for {
		data, _, _ := rf.SyncRX(bufferSize, bladerf.Metadata{})
		DataToAudioChannel(data)
	}
}

func Boot() {
	rf := ConfigureSDR()
	defer rf.Close()

	if os.Getenv("async") == "true" {
		Async(rf)
	} else {
		Sync(rf)
	}
}
