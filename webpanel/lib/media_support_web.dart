// ignore_for_file: avoid_web_libraries_in_flutter, deprecated_member_use

import 'dart:async';
import 'dart:html' as html;

class SelectedMediaFile {
  final html.File file;

  const SelectedMediaFile({required this.file});

  String get name => file.name;

  int get sizeBytes => file.size;
}

class MediaUploadResponse {
  final int statusCode;
  final String body;

  const MediaUploadResponse({required this.statusCode, required this.body});
}

Future<SelectedMediaFile?> chooseMp3File() async {
  final input = html.FileUploadInputElement()
    ..accept = '.mp3,audio/mpeg'
    ..multiple = false
    ..tabIndex = -1
    ..style.position = 'fixed'
    ..style.left = '-10000px'
    ..style.width = '1px'
    ..style.height = '1px'
    ..style.opacity = '0';
  final body = html.document.body;
  if (body == null) {
    throw StateError('The browser document is not ready for file selection');
  }
  body.append(input);

  final selection = Completer<SelectedMediaFile?>();
  StreamSubscription<html.Event>? changeSubscription;
  StreamSubscription<html.Event>? focusSubscription;
  Timer? cancellationCheck;
  var pickerOpened = false;

  void finish(SelectedMediaFile? file) {
    if (selection.isCompleted) return;
    cancellationCheck?.cancel();
    changeSubscription?.cancel();
    focusSubscription?.cancel();
    input.remove();
    selection.complete(file);
  }

  changeSubscription = input.onChange.listen((_) {
    final files = input.files;
    final file = files == null || files.isEmpty ? null : files.first;
    finish(file == null ? null : SelectedMediaFile(file: file));
  });
  focusSubscription = html.window.onFocus.listen((_) {
    if (!pickerOpened) return;
    cancellationCheck?.cancel();
    cancellationCheck = Timer(
      const Duration(milliseconds: 400),
      () => finish(null),
    );
  });

  pickerOpened = true;
  input.click();
  return selection.future;
}

Future<MediaUploadResponse> uploadMp3File(
  Uri url,
  SelectedMediaFile file, {
  void Function(int sentBytes, int totalBytes)? onProgress,
}) {
  final request = html.HttpRequest();
  final response = Completer<MediaUploadResponse>();
  final formData = html.FormData();
  formData.appendBlob('file', file.file, file.name);

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
  request.onLoadEnd.listen((_) {
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
