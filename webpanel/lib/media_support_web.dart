// ignore_for_file: avoid_web_libraries_in_flutter, deprecated_member_use

import 'dart:async';
import 'dart:html' as html;
import 'dart:typed_data';

class SelectedMediaFile {
  final String name;
  final Uint8List bytes;

  const SelectedMediaFile({required this.name, required this.bytes});
}

Future<SelectedMediaFile?> chooseMp3File() async {
  final input = html.FileUploadInputElement()..accept = '.mp3,audio/mpeg';
  input.click();
  await input.onChange.first;

  final file = input.files?.isEmpty ?? true ? null : input.files!.first;
  if (file == null) {
    return null;
  }

  final reader = html.FileReader()..readAsArrayBuffer(file);
  await reader.onLoad.first;
  final result = reader.result;
  if (result is! ByteBuffer) {
    return null;
  }
  return SelectedMediaFile(name: file.name, bytes: result.asUint8List());
}

abstract class MediaPlayer {
  Stream<void> get onEnded;

  Future<void> play(String url);

  void pause();

  void dispose();
}

MediaPlayer createMediaPlayer() => _WebMediaPlayer();

class _WebMediaPlayer implements MediaPlayer {
  final StreamController<void> _ended = StreamController<void>.broadcast();
  html.AudioElement? _audio;
  StreamSubscription<html.Event>? _endedSubscription;

  @override
  Stream<void> get onEnded => _ended.stream;

  @override
  Future<void> play(String url) async {
    if (_audio == null || _audio!.src != url) {
      await _endedSubscription?.cancel();
      _audio?.pause();
      _audio = html.AudioElement(url)..preload = 'metadata';
      _endedSubscription = _audio!.onEnded.listen((_) => _ended.add(null));
    }
    await _audio!.play();
  }

  @override
  void pause() {
    _audio?.pause();
  }

  @override
  void dispose() {
    _audio?.pause();
    _endedSubscription?.cancel();
    _ended.close();
  }
}
