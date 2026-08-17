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

## Narrow FM Listen Configuration

The listen client demodulates narrow FM locally and sends gated mono PCM to the server. The following `[listen]` settings control its RTL-SDR and DSP path:

| Setting | Meaning |
| --- | --- |
| `DeviceIndex`, `DirectSampling` | RTL-SDR device selection and direct-sampling mode. |
| `Frequency`, `SampleRate`, `AudioRate` | Receive frequency, IQ input rate, and PCM output rate. `SampleRate` must be divisible by `AudioRate`. |
| `TunerBandwidth`, `TunerGain` | RTL-SDR tuner settings. |
| `Bandwidth` | Optional digital channel bandwidth in Hz. `0` disables the IQ channel filter. |
| `FMDeviation`, `Tau` | FM deviation in Hz and de-emphasis time constant in microseconds. |
| `SquelchThreshold` | Manual dBFS threshold. A negative value enables manual squelch; `0` selects SNR-based squelch. |
| `SquelchSNRThreshold` | Automatic squelch threshold above the tracked noise floor in dB. `0` leaves carrier squelch open. |
| `CTCSS` | Required CTCSS tone in Hz. `0` disables tone gating. |
| `Notch`, `NotchQ` | Optional output notch frequency and Q. Use the configured CTCSS tone as `Notch` to remove it from transmitted audio. |
| `AmpFactor`, `AudioGain` | Floating-point DSP gain and final PCM gain. |
| `Highpass`, `Lowpass` | Output-audio filter cutoffs in Hz. Set either to `0` to disable that filter. |

Existing listener configurations migrate `SquelchDB`, `AudioCutoffHz`, and `DeemphasisTauUs` into their corresponding NFM settings when the new keys are first added. `TunerBandwidth` remains the hardware tuner setting; the new digital `Bandwidth` filter defaults to disabled. The old migrated keys may remain in the INI file but are no longer read by the listener.

## Updates

The clearlink executable auto-updates after running.

It checks for newer releases in the background, installs updates, and restarts with the new version.
