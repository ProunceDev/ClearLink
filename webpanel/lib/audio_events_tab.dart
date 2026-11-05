part of 'main.dart';

class AudioEventsTab extends StatefulWidget {
  final AdminApi api;
  final List<NodeConfigViewModel> nodes;

  const AudioEventsTab({super.key, required this.api, required this.nodes});

  @override
  State<AudioEventsTab> createState() => _AudioEventsTabState();
}

class _AudioEventsTabState extends State<AudioEventsTab> {
  List<ServerAudioEvent> _events = const [];
  List<MediaAsset> _media = const [];
  String? _errorMessage;
  String? _startingEventId;
  bool _isLoading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final eventsFuture = widget.api.fetchAudioEvents();
      final mediaFuture = widget.api.fetchMedia();
      final events = await eventsFuture;
      final media = await mediaFuture;
      if (!mounted) return;
      setState(() {
        _events = events;
        _media = media;
        _errorMessage = null;
        _isLoading = false;
      });
    } on AdminApiException catch (error) {
      if (!mounted) return;
      setState(() {
        _errorMessage = error.message;
        _isLoading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _errorMessage = 'Unable to load audio events';
        _isLoading = false;
      });
    }
  }

  Future<void> _edit([ServerAudioEvent? event]) async {
    final saved = await showDialog<ServerAudioEvent>(
      context: context,
      builder: (_) => AudioEventEditorDialog(
        api: widget.api,
        nodes: widget.nodes,
        media: _media,
        event: event,
      ),
    );
    if (!mounted || saved == null) return;
    await _load();
    if (!mounted) return;
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text('${saved.name} saved')));
  }

  Future<void> _toggle(ServerAudioEvent event, bool enabled) async {
    final originalEvents = _events;
    setState(() {
      _events = _events
          .map(
            (item) =>
                item.id == event.id ? event.copyWith(enabled: enabled) : item,
          )
          .toList();
    });
    try {
      await widget.api.saveAudioEvent(event.copyWith(enabled: enabled));
    } on AdminApiException catch (error) {
      if (!mounted) return;
      setState(() => _events = originalEvents);
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    } catch (_) {
      if (!mounted) return;
      setState(() => _events = originalEvents);
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Unable to update audio event')),
      );
    }
  }

  Future<void> _playNow(ServerAudioEvent event) async {
    if (_startingEventId != null) return;
    setState(() => _startingEventId = event.id);
    try {
      await widget.api.playAudioEvent(event.id);
      if (!mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('${event.name} playback started')));
    } on AdminApiException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Unable to start audio event playback')),
      );
    } finally {
      if (mounted) {
        setState(() => _startingEventId = null);
      }
    }
  }

  Future<void> _delete(ServerAudioEvent event) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        backgroundColor: const Color(0xFF101A2E),
        title: const Text(
          'Delete audio event',
          style: TextStyle(color: Colors.white),
        ),
        content: Text(
          'Delete ${event.name}?',
          style: const TextStyle(color: Color(0xFFA9B7C9)),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            style: FilledButton.styleFrom(
              backgroundColor: const Color(0xFFDC4C4C),
            ),
            onPressed: () => Navigator.of(dialogContext).pop(true),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;

    try {
      await widget.api.deleteAudioEvent(event.id);
      if (!mounted) return;
      setState(
        () => _events = _events.where((item) => item.id != event.id).toList(),
      );
    } on AdminApiException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Unable to delete audio event')),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) {
      return const Center(child: CircularProgressIndicator());
    }

    final mediaByID = {for (final asset in _media) asset.id: asset};
    return RefreshIndicator(
      onRefresh: _load,
      child: CustomScrollView(
        physics: const AlwaysScrollableScrollPhysics(),
        slivers: [
          SliverToBoxAdapter(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(24, 24, 24, 12),
              child: Row(
                children: [
                  const Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          'Audio events',
                          style: TextStyle(
                            color: Colors.white,
                            fontSize: 22,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        SizedBox(height: 4),
                        Text(
                          'IDs, announcements, and transmission cues.',
                          style: TextStyle(color: Color(0xFFA9B7C9)),
                        ),
                      ],
                    ),
                  ),
                  Tooltip(
                    message: 'Refresh audio events',
                    child: IconButton(
                      onPressed: _load,
                      icon: const Icon(Icons.refresh_outlined),
                    ),
                  ),
                  Tooltip(
                    message: 'Create audio event',
                    child: IconButton.filled(
                      onPressed: () => _edit(),
                      icon: const Icon(Icons.add),
                      style: IconButton.styleFrom(
                        backgroundColor: const Color(0xFF2DD4BF),
                        foregroundColor: const Color(0xFF07111B),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
          if (_errorMessage != null)
            SliverFillRemaining(
              hasScrollBody: false,
              child: _AdminEmptyState(
                icon: Icons.cloud_off_outlined,
                title: 'Audio events unavailable',
                message: _errorMessage!,
                action: OutlinedButton.icon(
                  onPressed: _load,
                  icon: const Icon(Icons.refresh_outlined),
                  label: const Text('Retry'),
                ),
              ),
            )
          else if (_events.isEmpty)
            SliverFillRemaining(
              hasScrollBody: false,
              child: _AdminEmptyState(
                icon: Icons.notifications_none_outlined,
                title: 'No audio events',
                message:
                    'Create an event to schedule IDs, announcements, or beeps.',
                action: FilledButton.icon(
                  onPressed: () => _edit(),
                  icon: const Icon(Icons.add),
                  label: const Text('Create event'),
                ),
              ),
            )
          else
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 32),
              sliver: SliverList.separated(
                itemCount: _events.length,
                separatorBuilder: (_, _) => const SizedBox(height: 10),
                itemBuilder: (context, index) {
                  final event = _events[index];
                  return _AudioEventCard(
                    event: event,
                    mediaByID: mediaByID,
                    onEdit: () => _edit(event),
                    onDelete: () => _delete(event),
                    onEnabledChanged: (value) => _toggle(event, value),
                    isStarting: _startingEventId == event.id,
                    isPlayDisabled: _startingEventId != null,
                    onPlayNow: () => _playNow(event),
                  );
                },
              ),
            ),
        ],
      ),
    );
  }
}

class _AudioEventCard extends StatelessWidget {
  final ServerAudioEvent event;
  final Map<String, MediaAsset> mediaByID;
  final VoidCallback onEdit;
  final VoidCallback onDelete;
  final ValueChanged<bool> onEnabledChanged;
  final bool isStarting;
  final bool isPlayDisabled;
  final VoidCallback onPlayNow;

  const _AudioEventCard({
    required this.event,
    required this.mediaByID,
    required this.onEdit,
    required this.onDelete,
    required this.onEnabledChanged,
    required this.isStarting,
    required this.isPlayDisabled,
    required this.onPlayNow,
  });

  @override
  Widget build(BuildContext context) {
    final mediaNames = event.mediaIds
        .map((id) => mediaByID[id]?.name ?? 'Missing MP3')
        .join(', ');
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF111D2F),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
      ),
      child: Wrap(
        spacing: 16,
        runSpacing: 12,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          Container(
            width: 38,
            height: 38,
            decoration: BoxDecoration(
              color: _audioEventColor(event.type),
              borderRadius: BorderRadius.circular(6),
            ),
            child: Icon(_audioEventIcon(event.type), color: Colors.white),
          ),
          SizedBox(
            width: 250,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  event.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 16,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  _audioEventTypeLabel(event.type),
                  style: const TextStyle(
                    color: Color(0xFF67E8F9),
                    fontSize: 12,
                  ),
                ),
              ],
            ),
          ),
          SizedBox(
            width: 190,
            child: Text(
              _audioEventTimingLabel(event),
              style: const TextStyle(color: Color(0xFFA9B7C9), fontSize: 13),
            ),
          ),
          SizedBox(
            width: 240,
            child: Text(
              mediaNames,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(color: Colors.white70, fontSize: 12),
            ),
          ),
          Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Switch.adaptive(
                value: event.enabled,
                onChanged: onEnabledChanged,
              ),
              Tooltip(
                message: isStarting
                    ? 'Starting ${event.name}'
                    : 'Play ${event.name} now',
                child: IconButton.filled(
                  onPressed: isPlayDisabled ? null : onPlayNow,
                  icon: isStarting
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.play_arrow),
                  style: IconButton.styleFrom(
                    backgroundColor: const Color(0xFF2DD4BF),
                    foregroundColor: const Color(0xFF07111B),
                  ),
                ),
              ),
              Tooltip(
                message: 'Edit ${event.name}',
                child: IconButton(
                  onPressed: onEdit,
                  icon: const Icon(Icons.edit_outlined),
                ),
              ),
              Tooltip(
                message: 'Delete ${event.name}',
                child: IconButton(
                  onPressed: onDelete,
                  color: const Color(0xFFFCA5A5),
                  icon: const Icon(Icons.delete_outline),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class AudioEventEditorDialog extends StatefulWidget {
  final AdminApi api;
  final List<NodeConfigViewModel> nodes;
  final List<MediaAsset> media;
  final ServerAudioEvent? event;

  const AudioEventEditorDialog({
    super.key,
    required this.api,
    required this.nodes,
    required this.media,
    this.event,
  });

  @override
  State<AudioEventEditorDialog> createState() => _AudioEventEditorDialogState();
}

class _AudioEventEditorDialogState extends State<AudioEventEditorDialog> {
  late final TextEditingController _nameController;
  late final TextEditingController _intervalController;
  late String _type;
  late bool _enabled;
  late String _scheduleAt;
  late String _sourceNodeType;
  late int _sourcePeerId;
  late Set<String> _mediaIds;
  String? _errorMessage;
  bool _isSaving = false;

  @override
  void initState() {
    super.initState();
    final event = widget.event ?? ServerAudioEvent.empty();
    _nameController = TextEditingController(text: event.name);
    _intervalController = TextEditingController(
      text: ((event.intervalSeconds <= 0 ? 900 : event.intervalSeconds) / 60)
          .round()
          .toString(),
    );
    _type = event.type;
    _enabled = event.enabled;
    _scheduleAt = event.scheduleAt;
    _sourceNodeType = event.sourceNodeType;
    _sourcePeerId = event.sourcePeerId;
    _mediaIds = event.mediaIds.toSet();
  }

  @override
  void dispose() {
    _nameController.dispose();
    _intervalController.dispose();
    super.dispose();
  }

  bool get _isSourceEvent => _type == 'roger_beep' || _type == 'pre_roger_beep';

  List<NodeConfigViewModel> get _eligibleSourceNodes {
    return widget.nodes
        .where(
          (node) =>
              _sourceNodeType == 'all' || node.nodeType == _sourceNodeType,
        )
        .toList();
  }

  Future<void> _selectMedia() async {
    final selected = await showDialog<List<String>>(
      context: context,
      builder: (_) => _AudioEventMediaPicker(
        media: widget.media,
        selectedMediaIds: _mediaIds,
      ),
    );
    if (selected == null || !mounted) return;
    setState(() => _mediaIds = selected.toSet());
  }

  Future<void> _selectSchedule() async {
    final initial = _timeFromSchedule(_scheduleAt) ?? TimeOfDay.now();
    final selected = await showTimePicker(
      context: context,
      initialTime: initial,
    );
    if (selected == null || !mounted) return;
    setState(() {
      _scheduleAt =
          '${selected.hour.toString().padLeft(2, '0')}:${selected.minute.toString().padLeft(2, '0')}';
    });
  }

  Future<void> _save() async {
    final name = _nameController.text.trim();
    if (name.isEmpty) {
      setState(() => _errorMessage = 'Enter a name for this event');
      return;
    }
    if (_mediaIds.isEmpty) {
      setState(() => _errorMessage = 'Select at least one MP3 file');
      return;
    }
    final intervalMinutes = int.tryParse(_intervalController.text.trim()) ?? 0;
    if (_type == 'repeater_id' && intervalMinutes < 1) {
      setState(
        () => _errorMessage = 'Use an idle interval of at least one minute',
      );
      return;
    }
    if (_type == 'announcement' && _scheduleAt.isEmpty) {
      setState(() => _errorMessage = 'Select an announcement time');
      return;
    }

    setState(() {
      _isSaving = true;
      _errorMessage = null;
    });
    final existing = widget.event ?? ServerAudioEvent.empty();
    final event = ServerAudioEvent(
      id: existing.id,
      name: name,
      type: _type,
      enabled: _enabled,
      mediaIds: _mediaIds.toList()..sort(),
      intervalSeconds: _type == 'repeater_id' ? intervalMinutes * 60 : 0,
      scheduleAt: _type == 'announcement' ? _scheduleAt : '',
      sourcePeerId: _isSourceEvent ? _sourcePeerId : 0,
      sourceNodeType: _isSourceEvent ? _sourceNodeType : 'all',
    );
    try {
      final saved = await widget.api.saveAudioEvent(event);
      if (!mounted) return;
      Navigator.of(context).pop(saved);
    } on AdminApiException catch (error) {
      if (!mounted) return;
      setState(() => _errorMessage = error.message);
    } catch (_) {
      if (!mounted) return;
      setState(() => _errorMessage = 'Unable to save audio event');
    } finally {
      if (mounted) {
        setState(() => _isSaving = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final selectedMedia = widget.media
        .where((asset) => _mediaIds.contains(asset.id))
        .toList();
    final sourceNodes = _eligibleSourceNodes;
    final sourcePeerId = sourceNodes.any((node) => node.peerId == _sourcePeerId)
        ? _sourcePeerId
        : 0;

    return AlertDialog(
      backgroundColor: const Color(0xFF101A2E),
      title: Text(
        widget.event == null ? 'Create audio event' : 'Edit audio event',
        style: const TextStyle(color: Colors.white),
      ),
      content: SizedBox(
        width: 560,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              TextField(
                controller: _nameController,
                style: const TextStyle(color: Colors.white),
                decoration: const InputDecoration(labelText: 'Event name'),
              ),
              const SizedBox(height: 16),
              DropdownButtonFormField<String>(
                key: ValueKey('audio-event-type-$_type'),
                initialValue: _type,
                decoration: const InputDecoration(labelText: 'Event type'),
                items: const [
                  DropdownMenuItem(
                    value: 'repeater_id',
                    child: Text('Repeater identification'),
                  ),
                  DropdownMenuItem(
                    value: 'announcement',
                    child: Text('Scheduled announcement'),
                  ),
                  DropdownMenuItem(
                    value: 'roger_beep',
                    child: Text('Roger beep'),
                  ),
                  DropdownMenuItem(
                    value: 'pre_roger_beep',
                    child: Text('Pre-roger beep'),
                  ),
                ],
                onChanged: (value) {
                  setState(() {
                    _type = value ?? 'repeater_id';
                    if (!_isSourceEvent) {
                      _sourceNodeType = 'all';
                      _sourcePeerId = 0;
                    }
                  });
                },
              ),
              const SizedBox(height: 8),
              SwitchListTile.adaptive(
                contentPadding: EdgeInsets.zero,
                title: const Text(
                  'Enabled',
                  style: TextStyle(color: Colors.white),
                ),
                value: _enabled,
                onChanged: (value) => setState(() => _enabled = value),
              ),
              if (_type == 'repeater_id') ...[
                const SizedBox(height: 8),
                TextField(
                  controller: _intervalController,
                  keyboardType: TextInputType.number,
                  style: const TextStyle(color: Colors.white),
                  decoration: const InputDecoration(
                    labelText: 'Idle interval (minutes)',
                    helperText:
                        'Defaults to 15 minutes after broadcast activity.',
                  ),
                ),
              ],
              if (_type == 'announcement') ...[
                const SizedBox(height: 12),
                OutlinedButton.icon(
                  onPressed: _selectSchedule,
                  icon: const Icon(Icons.schedule_outlined),
                  label: Text(
                    _scheduleAt.isEmpty
                        ? 'Select time'
                        : 'Scheduled for $_scheduleAt',
                  ),
                ),
              ],
              if (_isSourceEvent) ...[
                const SizedBox(height: 16),
                DropdownButtonFormField<String>(
                  key: ValueKey('audio-event-source-$_sourceNodeType'),
                  initialValue: _sourceNodeType,
                  decoration: const InputDecoration(
                    labelText: 'Transmission source type',
                  ),
                  items: const [
                    DropdownMenuItem(value: 'all', child: Text('Any source')),
                    DropdownMenuItem(
                      value: 'listen',
                      child: Text('Listen source'),
                    ),
                    DropdownMenuItem(
                      value: 'interface',
                      child: Text('Interface source'),
                    ),
                  ],
                  onChanged: (value) {
                    setState(() {
                      _sourceNodeType = value ?? 'all';
                      _sourcePeerId = 0;
                    });
                  },
                ),
                const SizedBox(height: 12),
                DropdownButtonFormField<int>(
                  key: ValueKey('audio-event-source-peer-$sourcePeerId'),
                  initialValue: sourcePeerId,
                  decoration: const InputDecoration(
                    labelText: 'Specific source node',
                  ),
                  items: [
                    const DropdownMenuItem(
                      value: 0,
                      child: Text('Any matching node'),
                    ),
                    for (final node in sourceNodes)
                      DropdownMenuItem(
                        value: node.peerId,
                        child: Text('${node.name} (#${node.peerId})'),
                      ),
                  ],
                  onChanged: (value) =>
                      setState(() => _sourcePeerId = value ?? 0),
                ),
              ],
              const SizedBox(height: 20),
              Row(
                children: [
                  const Expanded(
                    child: Text(
                      'MP3 files',
                      style: TextStyle(
                        color: Colors.white,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                  OutlinedButton.icon(
                    onPressed: _selectMedia,
                    icon: const Icon(Icons.library_music_outlined),
                    label: const Text('Select files'),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              if (widget.media.isEmpty)
                const Text(
                  'Upload MP3 files in the Media library tab first.',
                  style: TextStyle(color: Color(0xFFA9B7C9)),
                )
              else if (selectedMedia.isEmpty)
                const Text(
                  'No MP3 files selected.',
                  style: TextStyle(color: Color(0xFFA9B7C9)),
                )
              else
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: [
                    for (final asset in selectedMedia)
                      InputChip(
                        label: Text(asset.name),
                        onDeleted: () =>
                            setState(() => _mediaIds.remove(asset.id)),
                      ),
                  ],
                ),
              const SizedBox(height: 6),
              const Text(
                'One selected MP3 is chosen at random when this event plays.',
                style: TextStyle(color: Color(0xFF86EFAC), fontSize: 12),
              ),
              if (_errorMessage != null) ...[
                const SizedBox(height: 12),
                Text(
                  _errorMessage!,
                  style: const TextStyle(color: Color(0xFFFCA5A5)),
                ),
              ],
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: _isSaving ? null : () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton.icon(
          onPressed: _isSaving || widget.media.isEmpty ? null : _save,
          icon: _isSaving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.save_outlined),
          label: const Text('Save event'),
        ),
      ],
    );
  }
}

class _AudioEventMediaPicker extends StatefulWidget {
  final List<MediaAsset> media;
  final Set<String> selectedMediaIds;

  const _AudioEventMediaPicker({
    required this.media,
    required this.selectedMediaIds,
  });

  @override
  State<_AudioEventMediaPicker> createState() => _AudioEventMediaPickerState();
}

class _AudioEventMediaPickerState extends State<_AudioEventMediaPicker> {
  late final Set<String> _selected = {...widget.selectedMediaIds};

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: const Color(0xFF101A2E),
      title: const Text(
        'Select MP3 files',
        style: TextStyle(color: Colors.white),
      ),
      content: SizedBox(
        width: 480,
        height: 360,
        child: ListView.separated(
          itemCount: widget.media.length,
          separatorBuilder: (_, _) => const Divider(height: 1),
          itemBuilder: (context, index) {
            final asset = widget.media[index];
            final selected = _selected.contains(asset.id);
            return CheckboxListTile(
              value: selected,
              onChanged: (value) {
                setState(() {
                  if (value ?? false) {
                    _selected.add(asset.id);
                  } else {
                    _selected.remove(asset.id);
                  }
                });
              },
              title: Text(
                asset.name,
                style: const TextStyle(color: Colors.white),
              ),
              subtitle: Text(
                _formatBytes(asset.sizeBytes),
                style: const TextStyle(color: Color(0xFFA9B7C9)),
              ),
              controlAffinity: ListTileControlAffinity.leading,
            );
          },
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: () => Navigator.of(context).pop(_selected.toList()),
          child: const Text('Use selected'),
        ),
      ],
    );
  }
}

IconData _audioEventIcon(String type) {
  switch (type) {
    case 'repeater_id':
      return Icons.radio_outlined;
    case 'announcement':
      return Icons.campaign_outlined;
    case 'roger_beep':
      return Icons.volume_up_outlined;
    case 'pre_roger_beep':
      return Icons.volume_down_outlined;
    default:
      return Icons.audiotrack_outlined;
  }
}

Color _audioEventColor(String type) {
  switch (type) {
    case 'repeater_id':
      return const Color(0xFF0F766E);
    case 'announcement':
      return const Color(0xFF1D4ED8);
    case 'roger_beep':
      return const Color(0xFF9A3412);
    case 'pre_roger_beep':
      return const Color(0xFF7C3AED);
    default:
      return const Color(0xFF475569);
  }
}

String _audioEventTypeLabel(String type) {
  switch (type) {
    case 'repeater_id':
      return 'Repeater identification';
    case 'announcement':
      return 'Scheduled announcement';
    case 'roger_beep':
      return 'Roger beep';
    case 'pre_roger_beep':
      return 'Pre-roger beep';
    default:
      return 'Audio event';
  }
}

String _audioEventTimingLabel(ServerAudioEvent event) {
  switch (event.type) {
    case 'repeater_id':
      final minutes = (event.intervalSeconds / 60).round();
      return 'After $minutes min idle';
    case 'announcement':
      return event.scheduleAt.isEmpty
          ? 'Time not set'
          : 'At ${event.scheduleAt}';
    case 'roger_beep':
      return _audioEventSourceLabel(event, 'After transmission');
    case 'pre_roger_beep':
      return _audioEventSourceLabel(event, 'Before transmission');
    default:
      return '';
  }
}

String _audioEventSourceLabel(ServerAudioEvent event, String prefix) {
  if (event.sourcePeerId != 0) {
    return '$prefix from node #${event.sourcePeerId}';
  }
  if (event.sourceNodeType == 'all') return '$prefix from any source';
  return '$prefix from ${event.sourceNodeType}';
}

TimeOfDay? _timeFromSchedule(String value) {
  final parts = value.split(':');
  if (parts.length != 2) return null;
  final hour = int.tryParse(parts[0]);
  final minute = int.tryParse(parts[1]);
  if (hour == null || minute == null || hour > 23 || minute > 59) return null;
  return TimeOfDay(hour: hour, minute: minute);
}
