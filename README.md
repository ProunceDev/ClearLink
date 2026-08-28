# ClearLink

ClearLink is a radio connection network.

It connects radio link nodes over the internet so audio can be routed between locations.

## Install

1. Go to GitHub Releases for ClearLink.
2. Download the clearlink executable for your platform.
3. Put it in its own folder and make it executable on Linux.

Linux example:

```bash
chmod +x ./clearlink
./clearlink
```

## Run

Run the clearlink executable.

On first run, it walks you through setup and creates local config files.

After setup, it starts the selected ClearLink role automatically.

On Debian-based Linux systems, the manager installs `alsa-utils` when an Interface node or Radio Broadcast node needs missing `arecord` or `aplay` commands. It does not request package-install privileges when those commands are already available.

## Narrow FM Listen Configuration

The listen client demodulates narrow FM locally and sends gated mono PCM to the server. The following `[listen]` settings control its RTL-SDR and DSP path:

| Setting | Meaning |
| --- | --- |
| `DeviceIndex`, `DirectSampling` | RTL-SDR device selection and direct-sampling mode. |
| `Frequency`, `CenterFrequency`, `SampleRate`, `AudioRate` | Receive frequency, optional SDR center frequency, IQ input rate, and PCM output rate. Set `CenterFrequency` to `0` to use `Frequency`. `SampleRate` must be divisible by `AudioRate`. |
| `TunerBandwidth`, `TunerGain` | RTL-SDR tuner settings. |
| `Bandwidth` | Optional digital channel bandwidth in Hz. `0` disables the IQ channel filter. |
| `FMDeviation`, `Tau` | FM deviation in Hz and de-emphasis time constant in microseconds. |
| `SquelchThreshold` | Manual dBFS threshold. A negative value enables manual squelch; `0` selects SNR-based squelch. |
| `SquelchSNRThreshold` | Automatic squelch threshold above the tracked noise floor in dB. `0` leaves carrier squelch open. |
| `CTCSS` | Required CTCSS tone in Hz. `0` disables tone gating. |
| `Notch`, `NotchQ` | Optional output notch frequency and Q. Use the configured CTCSS tone as `Notch` to remove it from transmitted audio. |
| `AmpFactor`, `AudioGain` | Floating-point DSP gain and final PCM gain. |
| `Highpass`, `Lowpass` | Output-audio filter cutoffs in Hz. Set either to `0` to disable that filter. |
| `RouteToBroadcast`, `RouteToInterface` | Enable the node's RF audio for Broadcast and Interface destinations independently. |

Existing listener configurations migrate `SquelchDB`, `AudioCutoffHz`, and `DeemphasisTauUs` into their corresponding NFM settings when the new keys are first added. `TunerBandwidth` remains the hardware tuner setting; `CenterFrequency` controls the SDR center tune while the listener still demodulates at `Frequency`. The new digital `Bandwidth` filter defaults to disabled. The old migrated keys may remain in the INI file but are no longer read by the listener.

## Interface Node Configuration

An Interface node runs on a Raspberry Pi with an ALSA soundcard and two GPIO lines. It reads mono PCM from `InputDevice` while the `Squelch_Pin` input is active, and it plays routed audio through `OutputDevice` while asserting `Ptt_Pin`. `Squelch_Active_High` and `Ptt_Active_High` support active-low hardware without external inversion.

`arecord` and `aplay` from the `alsa-utils` package must be installed. `AudioRate` defaults to 48000 Hz and `AudioChunkMs` defaults to 20 ms; their product must produce an integral PCM frame.

On hosts that are not Raspberry Pi devices, Broadcast and Interface nodes skip GPIO access. Broadcast radio playback runs without PTT control; Interface input is treated as squelch-open and its PTT output is disabled.

All audio sources expose `RouteToBroadcast`; Listen nodes additionally expose `RouteToInterface`. These source flags decide which node classes receive their audio. By default, Listen routes to both classes and Interface routes to Broadcast. The server mixes every live Interface source with the selected RF Listen source for each eligible destination. Interface-origin audio is never routed into Interface playback, preventing soundcard feedback.

## Updates

The clearlink executable auto-updates after running.

It checks for newer releases in the background, installs updates, and restarts with the new version.
