import 'dart:async';
import 'dart:typed_data';

class SelectedMediaFile {
  final String name;
  final Uint8List bytes;

  const SelectedMediaFile({required this.name, required this.bytes});

  int get sizeBytes => bytes.length;
}

class MediaUploadResponse {
  final int statusCode;
  final String body;

  const MediaUploadResponse({required this.statusCode, required this.body});
}

Future<SelectedMediaFile?> chooseMp3File() async => null;

Future<MediaUploadResponse> uploadMp3File(
  Uri url,
  SelectedMediaFile file, {
  void Function(int sentBytes, int totalBytes)? onProgress,
}) async {
  throw UnsupportedError('MP3 uploads are available in the web panel only');
}

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
