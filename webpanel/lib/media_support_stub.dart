import 'dart:async';
import 'dart:typed_data';

class SelectedMediaFile {
  final String name;
  final Uint8List bytes;

  const SelectedMediaFile({required this.name, required this.bytes});
}

Future<SelectedMediaFile?> chooseMp3File() async => null;

abstract class MediaPlayer {
  Stream<void> get onEnded;

  Future<void> play(String url);

  void pause();

  void dispose();
}

MediaPlayer createMediaPlayer() => _StubMediaPlayer();

class _StubMediaPlayer implements MediaPlayer {
  @override
  Stream<void> get onEnded => const Stream<void>.empty();

  @override
  void dispose() {}

  @override
  void pause() {}

  @override
  Future<void> play(String url) async {}
}
