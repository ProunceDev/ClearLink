// ignore_for_file: avoid_web_libraries_in_flutter, deprecated_member_use

import 'dart:async';
import 'dart:html' as html;
import 'dart:typed_data';

class SelectedMediaFile {
  final String name;
  final Uint8List bytes;

  const SelectedMediaFile({required this.name, required this.bytes});
}

class MediaUploadResponse {
  final int statusCode;
  final String body;

  const MediaUploadResponse({required this.statusCode, required this.body});
}

Future<SelectedMediaFile?> chooseMp3File() async {
  final input = html.FileUploadInputElement()
    ..accept = '.mp3,audio/mpeg'
    ..style.display = 'none';
  html.document.body?.append(input);

  try {
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
  } finally {
    input.remove();
  }
}

Future<MediaUploadResponse> uploadMp3File(
  Uri url,
  SelectedMediaFile file, {
  void Function(int sentBytes, int totalBytes)? onProgress,
}) {
  final request = html.HttpRequest();
  final response = Completer<MediaUploadResponse>();
  final formData = html.FormData();
  formData.appendBlob('file', html.Blob([file.bytes], 'audio/mpeg'), file.name);

  request
    ..open('POST', url.toString())
    ..withCredentials = true;
  request.upload.onProgress.listen((event) {
    final loaded = event.loaded;
    final total = event.total;
    if (event.lengthComputable &&
        loaded != null &&
        total != null &&
        total > 0) {
      onProgress?.call(loaded, total);
    }
  });
  request.onLoad.listen((_) {
    if (!response.isCompleted) {
      response.complete(
        MediaUploadResponse(
          statusCode: request.status ?? 0,
          body: request.responseText ?? '',
        ),
      );
    }
  });
  request.onError.listen((_) {
    if (!response.isCompleted) {
      response.completeError(
        StateError('The upload connection was interrupted'),
      );
    }
  });
  request.onAbort.listen((_) {
    if (!response.isCompleted) {
      response.completeError(StateError('The upload was cancelled'));
    }
  });
  request.send(formData);
  return response.future;
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
