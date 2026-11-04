import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

import 'media_support.dart';

void main() {
  runApp(const ClearLinkAdminApp());
}

class ClearLinkAdminApp extends StatelessWidget {
  final TopologyResponse? initialTopology;

  const ClearLinkAdminApp({super.key, this.initialTopology});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'ClearLink Admin',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        brightness: Brightness.dark,
        scaffoldBackgroundColor: const Color(0xFF0B1220),
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF2DD4BF),
          brightness: Brightness.dark,
        ),
      ),
      home: DashboardPage(initialTopology: initialTopology),
    );
  }
}

class DashboardPage extends StatefulWidget {
  final TopologyResponse? initialTopology;

  const DashboardPage({super.key, this.initialTopology});

  @override
  State<DashboardPage> createState() => _DashboardPageState();
}

class _DashboardPageState extends State<DashboardPage> {
  _DashboardData? _dashboardData;
  String? _errorMessage;
  bool _isLoading = true;
  Timer? _refreshTimer;

  @override
  void initState() {
    super.initState();
    _loadDashboard();
    _refreshTimer = Timer.periodic(const Duration(milliseconds: 200), (_) {
      _refreshDashboard();
    });
  }

  @override
  void dispose() {
    _refreshTimer?.cancel();
    super.dispose();
  }

  Future<void> _loadDashboard() async {
    try {
      final data = await _fetchDashboardData();
      if (!mounted) return;
      setState(() {
        _dashboardData = data;
        _errorMessage = null;
        _isLoading = false;
      });
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _errorMessage = error.toString();
        _isLoading = false;
      });
    }
  }

  Future<void> _refreshDashboard() async {
    if (!mounted) return;
    try {
      final data = await _fetchDashboardData();
      if (!mounted) return;
      setState(() {
        _dashboardData = data;
        _errorMessage = null;
      });
    } catch (_) {
      // Ignore transient polling failures so the dashboard stays live without flicker.
    }
  }

  Future<_DashboardData> _fetchDashboardData() async {
    if (widget.initialTopology != null) {
      return _DashboardData(
        topology: widget.initialTopology!,
        connections: const [],
      );
    }

    final baseUrl = _getBaseUrl();
    final topologyRes = await http.get(
      Uri.parse('$baseUrl/api/public/topology'),
    );
    if (topologyRes.statusCode != 200) {
      throw Exception('Failed to load topology: ${topologyRes.statusCode}');
    }

    final topology = TopologyResponse.fromJson(jsonDecode(topologyRes.body));

    final connectionsRes = await http.get(
      Uri.parse('$baseUrl/api/public/connections'),
    );
    List<TopologyConnection> connections = const [];
    if (connectionsRes.statusCode == 200) {
      final decoded = jsonDecode(connectionsRes.body);
      if (decoded is Map<String, dynamic> && decoded['connections'] is List) {
        connections = (decoded['connections'] as List)
            .map(
              (entry) =>
                  TopologyConnection.fromJson(entry as Map<String, dynamic>),
            )
            .toList();
      }
    }

    return _DashboardData(topology: topology, connections: connections);
  }

  String _getBaseUrl() {
    final uri = Uri.base;
    final hasOrigin = uri.origin.isNotEmpty && uri.origin != 'null';
    return hasOrigin ? uri.origin : 'http://localhost:8080';
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: DecoratedBox(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [Color(0xFF0A1120), Color(0xFF101A2E)],
          ),
        ),
        child: SafeArea(
          child: () {
            if (_isLoading && _dashboardData == null) {
              return const Center(child: CircularProgressIndicator());
            }

            if (_errorMessage != null && _dashboardData == null) {
              return Center(
                child: Card(
                  color: const Color(0xFF1C2435),
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: Text(
                      'Unable to load dashboard\n$_errorMessage',
                      textAlign: TextAlign.center,
                    ),
                  ),
                ),
              );
            }

            final data =
                _dashboardData ??
                _DashboardData(
                  topology: TopologyResponse.empty(),
                  connections: const [],
                );

            return SingleChildScrollView(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const SizedBox(height: 12),
                    Wrap(
                      spacing: 12,
                      runSpacing: 12,
                      crossAxisAlignment: WrapCrossAlignment.center,
                      children: [
                        Text(
                          'Dashboard',
                          style: Theme.of(context).textTheme.headlineMedium
                              ?.copyWith(
                                fontWeight: FontWeight.w700,
                                color: Colors.white,
                                letterSpacing: 0.5,
                              ),
                        ),
                        _MetricBadge(
                          label: 'Listen',
                          value: '${data.topology.listen.length}',
                        ),
                        _MetricBadge(
                          label: 'Interface',
                          value: '${data.topology.interfaceNodes.length}',
                        ),
                        _MetricBadge(
                          label: 'Broadcast',
                          value: '${data.topology.broadcast.length}',
                        ),
                        _MetricBadge(
                          label: 'Active',
                          value: '${_countActive(data.topology)}',
                        ),
                        IconButton(
                          onPressed: _openAdminWorkspace,
                          tooltip: 'Admin panel',
                          style: IconButton.styleFrom(
                            backgroundColor: const Color(0xFF111B2B),
                            foregroundColor: Colors.white,
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(14),
                            ),
                            padding: const EdgeInsets.all(12),
                          ),
                          icon: const Icon(Icons.settings_outlined),
                        ),
                      ],
                    ),
                    const SizedBox(height: 24),
                    SizedBox(
                      height: 525,
                      child: TopologyDiagram(
                        topology: data.topology,
                        connections: data.connections,
                      ),
                    ),
                  ],
                ),
              ),
            );
          }(),
        ),
      ),
    );
  }

  Future<void> _openAdminWorkspace() async {
    final api = AdminApi(baseUrl: _getBaseUrl());
    try {
      final nodes = await api.fetchNodes();
      if (!mounted) return;
      await Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => AdminWorkspacePage(api: api, initialNodes: nodes),
        ),
      );
    } on AdminApiException catch (error) {
      if (!mounted) return;
      if (error.statusCode == 401) {
        _showAdminLogin(api);
        return;
      }
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Unable to reach the admin API')),
      );
    }
  }

  void _showAdminLogin(AdminApi api) {
    final usernameController = TextEditingController();
    final passwordController = TextEditingController();

    showDialog(
      context: context,
      builder: (dialogContext) {
        return AlertDialog(
          backgroundColor: const Color(0xFF101A2E),
          title: const Text(
            'Admin login',
            style: TextStyle(color: Colors.white),
          ),
          content: SizedBox(
            width: 360,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                TextField(
                  controller: usernameController,
                  autofocus: true,
                  style: const TextStyle(color: Colors.white),
                  decoration: const InputDecoration(
                    labelText: 'Username',
                    labelStyle: TextStyle(color: Color(0xFFA9B7C9)),
                  ),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: passwordController,
                  obscureText: true,
                  onSubmitted: (_) => _submitAdminLogin(
                    dialogContext,
                    api,
                    usernameController,
                    passwordController,
                  ),
                  style: const TextStyle(color: Colors.white),
                  decoration: const InputDecoration(
                    labelText: 'Password',
                    labelStyle: TextStyle(color: Color(0xFFA9B7C9)),
                  ),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: const Text('Cancel'),
            ),
            FilledButton(
              onPressed: () => _submitAdminLogin(
                dialogContext,
                api,
                usernameController,
                passwordController,
              ),
              child: const Text('Login'),
            ),
          ],
        );
      },
    ).whenComplete(() {
      usernameController.dispose();
      passwordController.dispose();
    });
  }

  Future<void> _submitAdminLogin(
    BuildContext dialogContext,
    AdminApi api,
    TextEditingController usernameController,
    TextEditingController passwordController,
  ) async {
    final username = usernameController.text.trim();
    final password = passwordController.text;
    if (username.isEmpty || password.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Username and password are required')),
      );
      return;
    }

    final dialogNavigator = Navigator.of(dialogContext);
    try {
      await api.login(username: username, password: password);
      if (!mounted) return;
      dialogNavigator.pop();
      await _openAdminWorkspace();
    } on AdminApiException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Unable to reach the admin API')),
      );
    }
  }
}

class _DashboardData {
  final TopologyResponse topology;
  final List<TopologyConnection> connections;

  const _DashboardData({required this.topology, required this.connections});
}

class AdminApiException implements Exception {
  final String message;
  final int? statusCode;

  const AdminApiException(this.message, {this.statusCode});

  @override
  String toString() => message;
}

class AdminApi {
  final String baseUrl;

  const AdminApi({required this.baseUrl});

  Uri _uri(String path) => Uri.parse('$baseUrl$path');

  Future<void> login({
    required String username,
    required String password,
  }) async {
    final response = await http.post(
      _uri('/api/admin/login'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'username': username, 'password': password}),
    );
    _ensureSuccess(response, const [200], 'Invalid admin credentials');
  }

  Future<void> logout() async {
    final response = await http.post(_uri('/api/admin/logout'));
    _ensureSuccess(response, const [200], 'Unable to log out');
  }

  Future<List<NodeConfigViewModel>> fetchNodes() async {
    final response = await http.get(_uri('/api/admin/nodes'));
    _ensureSuccess(response, const [200], 'Unable to load connected nodes');
    final decoded = jsonDecode(response.body);
    if (decoded is! List) {
      throw const AdminApiException('The admin API returned invalid node data');
    }
    return decoded
        .map(
          (entry) =>
              NodeConfigViewModel.fromJson(entry as Map<String, dynamic>),
        )
        .toList();
  }

  Future<void> _saveConfig(_PendingConfigChange change) async {
    final response = await http.post(
      _uri('/api/admin/config'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({
        'peerId': change.peerId,
        'key': change.key,
        'type': change.type,
        'value': change.value,
      }),
    );
    _ensureSuccess(response, const [200], 'Unable to save ${change.key}');
  }

  Future<BulkSaveSummary> saveBulkConfig({
    required String nodeType,
    required String key,
    required String value,
  }) async {
    final response = await http.post(
      _uri('/api/admin/config/bulk'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({'nodeType': nodeType, 'key': key, 'value': value}),
    );
    _ensureSuccess(response, const [200], 'Unable to apply bulk update');
    final decoded = jsonDecode(response.body);
    if (decoded is! Map<String, dynamic>) {
      throw const AdminApiException('The admin API returned an invalid result');
    }
    return BulkSaveSummary.fromJson(decoded);
  }

  Future<List<MediaAsset>> fetchMedia() async {
    final response = await http.get(_uri('/api/admin/media'));
    _ensureSuccess(response, const [200], 'Unable to load the media library');
    final decoded = jsonDecode(response.body);
    if (decoded is! List) {
      throw const AdminApiException(
        'The admin API returned invalid media data',
      );
    }
    return decoded
        .map((entry) => MediaAsset.fromJson(entry as Map<String, dynamic>))
        .toList();
  }

  Future<MediaAsset> uploadMedia(SelectedMediaFile file) async {
    final request = http.MultipartRequest('POST', _uri('/api/admin/media'));
    request.files.add(
      http.MultipartFile.fromBytes('file', file.bytes, filename: file.name),
    );
    final response = await http.Response.fromStream(await request.send());
    _ensureSuccess(response, const [201], 'Unable to upload ${file.name}');
    final decoded = jsonDecode(response.body);
    if (decoded is! Map<String, dynamic>) {
      throw const AdminApiException(
        'The admin API returned invalid media data',
      );
    }
    return MediaAsset.fromJson(decoded);
  }

  Future<void> deleteMedia(String mediaId) async {
    final response = await http.delete(_uri('/api/admin/media/$mediaId'));
    _ensureSuccess(response, const [204], 'Unable to delete media');
  }

  String streamUrl(MediaAsset asset) => '$baseUrl${asset.streamUrl}';

  void _ensureSuccess(
    http.Response response,
    Iterable<int> successStatuses,
    String fallback,
  ) {
    if (successStatuses.contains(response.statusCode)) {
      return;
    }
    final message = response.body.trim().isEmpty
        ? fallback
        : response.body.trim();
    throw AdminApiException(message, statusCode: response.statusCode);
  }
}

class BulkSaveSummary {
  final int attempted;
  final int updated;

  const BulkSaveSummary({required this.attempted, required this.updated});

  factory BulkSaveSummary.fromJson(Map<String, dynamic> json) {
    return BulkSaveSummary(
      attempted: (json['attempted'] as num?)?.toInt() ?? 0,
      updated: (json['updated'] as num?)?.toInt() ?? 0,
    );
  }
}

class MediaAsset {
  final String id;
  final String name;
  final int sizeBytes;
  final String uploadedAt;
  final String streamUrl;

  const MediaAsset({
    required this.id,
    required this.name,
    required this.sizeBytes,
    required this.uploadedAt,
    required this.streamUrl,
  });

  factory MediaAsset.fromJson(Map<String, dynamic> json) {
    return MediaAsset(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? 'Untitled MP3',
      sizeBytes: (json['sizeBytes'] as num?)?.toInt() ?? 0,
      uploadedAt: json['uploadedAt'] as String? ?? '',
      streamUrl: json['streamUrl'] as String? ?? '',
    );
  }
}

class AdminWorkspacePage extends StatefulWidget {
  final AdminApi api;
  final List<NodeConfigViewModel> initialNodes;

  const AdminWorkspacePage({
    super.key,
    required this.api,
    required this.initialNodes,
  });

  @override
  State<AdminWorkspacePage> createState() => _AdminWorkspacePageState();
}

class _AdminWorkspacePageState extends State<AdminWorkspacePage> {
  late List<NodeConfigViewModel> _nodes = widget.initialNodes;
  bool _isRefreshing = false;

  Future<void> _refreshNodes() async {
    if (_isRefreshing) return;
    setState(() => _isRefreshing = true);
    try {
      final nodes = await widget.api.fetchNodes();
      if (!mounted) return;
      setState(() => _nodes = nodes);
    } on AdminApiException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Unable to refresh connected nodes')),
      );
    } finally {
      if (mounted) {
        setState(() => _isRefreshing = false);
      }
    }
  }

  Future<void> _logout() async {
    try {
      await widget.api.logout();
    } catch (_) {
      // The local browser session is cleared whenever the API is reachable.
    }
    if (mounted) {
      Navigator.of(context).pop();
    }
  }

  @override
  Widget build(BuildContext context) {
    return DefaultTabController(
      length: 2,
      child: Scaffold(
        backgroundColor: const Color(0xFF0A1120),
        appBar: AppBar(
          backgroundColor: const Color(0xFF101A2E),
          surfaceTintColor: Colors.transparent,
          elevation: 0,
          titleSpacing: 24,
          title: const Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                'Admin workspace',
                style: TextStyle(
                  color: Colors.white,
                  fontWeight: FontWeight.w700,
                ),
              ),
              Text(
                'ClearLink control plane',
                style: TextStyle(color: Color(0xFFA9B7C9), fontSize: 12),
              ),
            ],
          ),
          actions: [
            IconButton(
              onPressed: _isRefreshing ? null : _refreshNodes,
              tooltip: 'Refresh nodes',
              icon: _isRefreshing
                  ? const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.refresh_outlined),
            ),
            TextButton.icon(
              onPressed: _logout,
              icon: const Icon(Icons.logout_outlined, size: 18),
              label: const Text('Log out'),
              style: TextButton.styleFrom(foregroundColor: Colors.white70),
            ),
            const SizedBox(width: 12),
          ],
          bottom: const TabBar(
            tabs: [
              Tab(icon: Icon(Icons.tune_outlined), text: 'Configuration'),
              Tab(
                icon: Icon(Icons.library_music_outlined),
                text: 'Media library',
              ),
            ],
          ),
        ),
        body: DecoratedBox(
          decoration: const BoxDecoration(
            gradient: LinearGradient(
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
              colors: [Color(0xFF0B1220), Color(0xFF101A2E)],
            ),
          ),
          child: TabBarView(
            children: [
              AdminConfigTab(
                api: widget.api,
                nodes: _nodes,
                onRefresh: _refreshNodes,
              ),
              MediaLibraryTab(api: widget.api),
            ],
          ),
        ),
      ),
    );
  }
}

class AdminConfigTab extends StatefulWidget {
  final AdminApi api;
  final List<NodeConfigViewModel> nodes;
  final Future<void> Function() onRefresh;

  const AdminConfigTab({
    super.key,
    required this.api,
    required this.nodes,
    required this.onRefresh,
  });

  @override
  State<AdminConfigTab> createState() => _AdminConfigTabState();
}

class _AdminConfigTabState extends State<AdminConfigTab> {
  final Map<String, TextEditingController> _controllers = {};
  final Map<String, _PendingConfigChange> _pendingChanges = {};
  String _nodeFilter = 'all';
  int? _selectedPeerId;
  bool _isSaving = false;

  @override
  void initState() {
    super.initState();
    if (widget.nodes.isNotEmpty) {
      _selectedPeerId = widget.nodes.first.peerId;
    }
  }

  @override
  void didUpdateWidget(covariant AdminConfigTab oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.nodes.every((node) => node.peerId != _selectedPeerId)) {
      _selectedPeerId = widget.nodes.isEmpty ? null : widget.nodes.first.peerId;
    }
  }

  @override
  void dispose() {
    for (final controller in _controllers.values) {
      controller.dispose();
    }
    super.dispose();
  }

  List<NodeConfigViewModel> get _filteredNodes {
    if (_nodeFilter == 'all') return widget.nodes;
    return widget.nodes
        .where((node) => node.nodeType == _nodeFilter)
        .toList(growable: false);
  }

  NodeConfigViewModel? get _selectedNode {
    for (final node in widget.nodes) {
      if (node.peerId == _selectedPeerId) return node;
    }
    return null;
  }

  String _entryId(NodeConfigViewModel node, NodeConfigEntryViewModel entry) {
    return '${node.peerId}:${entry.type}:${entry.key}';
  }

  TextEditingController _controllerFor(
    NodeConfigViewModel node,
    NodeConfigEntryViewModel entry,
  ) {
    final id = _entryId(node, entry);
    return _controllers.putIfAbsent(
      id,
      () => TextEditingController(
        text: _pendingChanges[id]?.value ?? entry.value,
      ),
    );
  }

  String _valueFor(NodeConfigViewModel node, NodeConfigEntryViewModel entry) {
    return _pendingChanges[_entryId(node, entry)]?.value ?? entry.value;
  }

  void _queueChange(
    NodeConfigViewModel node,
    NodeConfigEntryViewModel entry,
    String value,
  ) {
    final id = _entryId(node, entry);
    setState(() {
      _pendingChanges[id] = _PendingConfigChange(
        id: id,
        peerId: node.peerId,
        key: entry.key,
        type: entry.type,
        value: value,
      );
    });
  }

  void _selectFilter(String filter) {
    final visibleNodes = filter == 'all'
        ? widget.nodes
        : widget.nodes
              .where((node) => node.nodeType == filter)
              .toList(growable: false);
    setState(() {
      _nodeFilter = filter;
      if (visibleNodes.every((node) => node.peerId != _selectedPeerId)) {
        _selectedPeerId = visibleNodes.isEmpty
            ? null
            : visibleNodes.first.peerId;
      }
    });
  }

  Future<void> _saveChanges() async {
    if (_pendingChanges.isEmpty || _isSaving) return;
    final changes = _pendingChanges.values.toList(growable: false);
    final savedIds = <String>[];
    final failures = <String>[];
    setState(() => _isSaving = true);

    for (final change in changes) {
      try {
        await widget.api._saveConfig(change);
        savedIds.add(change.id);
      } on AdminApiException catch (error) {
        failures.add('${change.key}: ${error.message}');
      } catch (_) {
        failures.add('${change.key}: unable to save');
      }
    }

    if (!mounted) return;
    setState(() {
      for (final id in savedIds) {
        _pendingChanges.remove(id);
      }
      _isSaving = false;
    });
    await widget.onRefresh();
    if (!mounted) return;

    final message = failures.isEmpty
        ? '${savedIds.length} setting${savedIds.length == 1 ? '' : 's'} saved'
        : '${savedIds.length} saved. ${failures.length} failed.';
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  Future<void> _openBulkEditor() async {
    final summary = await showDialog<BulkSaveSummary>(
      context: context,
      builder: (_) => BulkEditDialog(api: widget.api, nodes: widget.nodes),
    );
    if (!mounted || summary == null) return;
    await widget.onRefresh();
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(
          '${summary.updated} of ${summary.attempted} connected node settings updated',
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (widget.nodes.isEmpty) {
      return _AdminEmptyState(
        icon: Icons.settings_ethernet_outlined,
        title: 'No connected nodes',
        message:
            'Configuration controls appear when a ClearLink node connects.',
        action: OutlinedButton.icon(
          onPressed: widget.onRefresh,
          icon: const Icon(Icons.refresh_outlined),
          label: const Text('Refresh'),
        ),
      );
    }

    final selectedNode = _selectedNode;
    return LayoutBuilder(
      builder: (context, constraints) {
        final compact = constraints.maxWidth < 900;
        final navigator = _nodeNavigator();
        final editor = selectedNode == null
            ? const _AdminEmptyState(
                icon: Icons.touch_app_outlined,
                title: 'Select a node',
                message: 'Choose a connected node to review its settings.',
              )
            : _settingsEditor(selectedNode);

        if (compact) {
          return Column(
            children: [
              SizedBox(height: 250, child: navigator),
              Expanded(child: editor),
            ],
          );
        }

        return Row(
          children: [
            SizedBox(width: 300, child: navigator),
            Expanded(child: editor),
          ],
        );
      },
    );
  }

  Widget _nodeNavigator() {
    final nodes = _filteredNodes;
    return Container(
      decoration: BoxDecoration(
        color: const Color(0xFF0D1728).withValues(alpha: 0.88),
        border: Border(
          right: BorderSide(color: Colors.white.withValues(alpha: 0.08)),
          bottom: BorderSide(color: Colors.white.withValues(alpha: 0.08)),
        ),
      ),
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Expanded(
                child: Text(
                  'Connected nodes',
                  style: TextStyle(
                    color: Colors.white,
                    fontSize: 16,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              Tooltip(
                message: 'Bulk edit matching nodes',
                child: IconButton(
                  onPressed: _openBulkEditor,
                  icon: const Icon(Icons.tune_outlined),
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          Wrap(
            spacing: 6,
            runSpacing: 6,
            children: [
              for (final filter in const [
                'all',
                'listen',
                'interface',
                'broadcast',
              ])
                ChoiceChip(
                  label: Text(_nodeTypeLabel(filter)),
                  selected: _nodeFilter == filter,
                  onSelected: (_) => _selectFilter(filter),
                ),
            ],
          ),
          const SizedBox(height: 12),
          Expanded(
            child: nodes.isEmpty
                ? const Center(
                    child: Text(
                      'No matching nodes',
                      style: TextStyle(color: Colors.white54),
                    ),
                  )
                : ListView.separated(
                    itemCount: nodes.length,
                    separatorBuilder: (_, _) => const SizedBox(height: 6),
                    itemBuilder: (context, index) {
                      final node = nodes[index];
                      final selected = node.peerId == _selectedPeerId;
                      return Material(
                        color: selected
                            ? const Color(0xFF173247)
                            : Colors.transparent,
                        borderRadius: BorderRadius.circular(8),
                        child: InkWell(
                          borderRadius: BorderRadius.circular(8),
                          onTap: () =>
                              setState(() => _selectedPeerId = node.peerId),
                          child: Padding(
                            padding: const EdgeInsets.all(12),
                            child: Row(
                              children: [
                                Container(
                                  width: 8,
                                  height: 8,
                                  decoration: BoxDecoration(
                                    color: node.active
                                        ? const Color(0xFF2DD4BF)
                                        : const Color(0xFF69778B),
                                    shape: BoxShape.circle,
                                  ),
                                ),
                                const SizedBox(width: 10),
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        node.name,
                                        maxLines: 1,
                                        overflow: TextOverflow.ellipsis,
                                        style: const TextStyle(
                                          color: Colors.white,
                                          fontWeight: FontWeight.w600,
                                        ),
                                      ),
                                      const SizedBox(height: 3),
                                      Text(
                                        _nodeTypeLabel(node.nodeType),
                                        style: const TextStyle(
                                          color: Color(0xFFA9B7C9),
                                          fontSize: 11,
                                        ),
                                      ),
                                    ],
                                  ),
                                ),
                                Text(
                                  '#${node.peerId}',
                                  style: const TextStyle(
                                    color: Colors.white54,
                                    fontSize: 11,
                                  ),
                                ),
                              ],
                            ),
                          ),
                        ),
                      );
                    },
                  ),
          ),
        ],
      ),
    );
  }

  Widget _settingsEditor(NodeConfigViewModel node) {
    return Scrollbar(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 980),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Wrap(
                spacing: 16,
                runSpacing: 12,
                crossAxisAlignment: WrapCrossAlignment.center,
                children: [
                  Text(
                    node.name,
                    style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                      color: Colors.white,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  _NodeStatusPill(active: node.active),
                  _AdminDetail(label: 'Peer ID', value: node.peerId.toString()),
                  _AdminDetail(
                    label: 'Type',
                    value: _nodeTypeLabel(node.nodeType),
                  ),
                  _AdminDetail(label: 'Address', value: node.remoteAddr),
                  if (node.lastHeartbeatAgo.isNotEmpty)
                    _AdminDetail(
                      label: 'Heartbeat',
                      value: node.lastHeartbeatAgo,
                    ),
                ],
              ),
              const SizedBox(height: 28),
              Row(
                children: [
                  const Expanded(
                    child: Text(
                      'Configuration',
                      style: TextStyle(
                        color: Colors.white,
                        fontSize: 18,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                  if (_pendingChanges.isNotEmpty)
                    Text(
                      '${_pendingChanges.length} pending',
                      style: const TextStyle(
                        color: Color(0xFF86EFAC),
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  const SizedBox(width: 12),
                  FilledButton.icon(
                    onPressed: _pendingChanges.isEmpty || _isSaving
                        ? null
                        : _saveChanges,
                    icon: _isSaving
                        ? const SizedBox(
                            width: 16,
                            height: 16,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(Icons.save_outlined),
                    label: const Text('Save changes'),
                  ),
                ],
              ),
              const SizedBox(height: 10),
              const Text(
                'Changes are staged locally until you save them.',
                style: TextStyle(color: Color(0xFFA9B7C9), fontSize: 13),
              ),
              const SizedBox(height: 16),
              if (node.config.isEmpty)
                const _AdminEmptyState(
                  icon: Icons.settings_outlined,
                  title: 'No editable settings',
                  message: 'This node did not report configurable values.',
                )
              else
                for (final entry in node.config) _configEditor(node, entry),
            ],
          ),
        ),
      ),
    );
  }

  Widget _configEditor(
    NodeConfigViewModel node,
    NodeConfigEntryViewModel entry,
  ) {
    final isBoolean = entry.valueType == 'bool';
    final currentValue = _valueFor(node, entry);
    return Container(
      margin: const EdgeInsets.only(bottom: 10),
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF111D2F),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
      ),
      child: LayoutBuilder(
        builder: (context, constraints) {
          final field = isBoolean
              ? Switch.adaptive(
                  value: currentValue.toLowerCase() == 'true',
                  onChanged: (value) =>
                      _queueChange(node, entry, value.toString()),
                )
              : SizedBox(
                  width: constraints.maxWidth < 620 ? double.infinity : 320,
                  child: TextFormField(
                    controller: _controllerFor(node, entry),
                    keyboardType: _keyboardTypeFor(entry),
                    onChanged: (value) => _queueChange(node, entry, value),
                    style: const TextStyle(color: Colors.white),
                    decoration: InputDecoration(
                      labelText: _settingLabel(entry.key),
                      helperText: entry.valueType,
                      filled: true,
                      fillColor: const Color(0xFF0B1423),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(6),
                      ),
                    ),
                  ),
                );

          final details = Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                _settingLabel(entry.key),
                style: const TextStyle(
                  color: Colors.white,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                '${entry.valueType} setting',
                style: const TextStyle(color: Color(0xFFA9B7C9), fontSize: 12),
              ),
            ],
          );

          if (constraints.maxWidth < 620) {
            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [details, const SizedBox(height: 14), field],
            );
          }
          return Row(
            children: [
              Expanded(child: details),
              const SizedBox(width: 20),
              field,
            ],
          );
        },
      ),
    );
  }
}

class _PendingConfigChange {
  final String id;
  final int peerId;
  final String key;
  final String type;
  final String value;

  const _PendingConfigChange({
    required this.id,
    required this.peerId,
    required this.key,
    required this.type,
    required this.value,
  });
}

class BulkEditDialog extends StatefulWidget {
  final AdminApi api;
  final List<NodeConfigViewModel> nodes;

  const BulkEditDialog({super.key, required this.api, required this.nodes});

  @override
  State<BulkEditDialog> createState() => _BulkEditDialogState();
}

class _BulkEditDialogState extends State<BulkEditDialog> {
  final TextEditingController _valueController = TextEditingController();
  String _nodeType = 'all';
  String? _selectedKey;
  String? _errorMessage;
  bool _isSaving = false;

  @override
  void initState() {
    super.initState();
    _resetSelectedSetting();
  }

  @override
  void dispose() {
    _valueController.dispose();
    super.dispose();
  }

  List<NodeConfigEntryViewModel> get _settings {
    final entries = <String, NodeConfigEntryViewModel>{};
    for (final node in widget.nodes) {
      if (_nodeType != 'all' && node.nodeType != _nodeType) continue;
      for (final entry in node.config) {
        entries.putIfAbsent(entry.key, () => entry);
      }
    }
    final result = entries.values.toList();
    result.sort((first, second) => first.key.compareTo(second.key));
    return result;
  }

  NodeConfigEntryViewModel? get _selectedSetting {
    for (final entry in _settings) {
      if (entry.key == _selectedKey) return entry;
    }
    return null;
  }

  int get _targetCount => widget.nodes
      .where((node) => _nodeType == 'all' || node.nodeType == _nodeType)
      .length;

  void _resetSelectedSetting() {
    final first = _settings.isEmpty ? null : _settings.first;
    _selectedKey = first?.key;
    _valueController.text = first?.value ?? '';
  }

  Future<void> _save() async {
    final setting = _selectedSetting;
    if (setting == null || _isSaving) return;
    setState(() {
      _isSaving = true;
      _errorMessage = null;
    });
    try {
      final result = await widget.api.saveBulkConfig(
        nodeType: _nodeType,
        key: setting.key,
        value: _valueController.text,
      );
      if (!mounted) return;
      Navigator.of(context).pop(result);
    } on AdminApiException catch (error) {
      if (!mounted) return;
      setState(() => _errorMessage = error.message);
    } catch (_) {
      if (!mounted) return;
      setState(() => _errorMessage = 'Unable to apply the bulk update');
    } finally {
      if (mounted) {
        setState(() => _isSaving = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final setting = _selectedSetting;
    return AlertDialog(
      backgroundColor: const Color(0xFF101A2E),
      title: const Text(
        'Bulk edit settings',
        style: TextStyle(color: Colors.white),
      ),
      content: SizedBox(
        width: 460,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                'Apply one value to every connected node that exposes the selected setting.',
                style: TextStyle(color: Color(0xFFA9B7C9)),
              ),
              const SizedBox(height: 20),
              DropdownButtonFormField<String>(
                key: ValueKey('node-type-$_nodeType'),
                initialValue: _nodeType,
                decoration: const InputDecoration(labelText: 'Node type'),
                items: const [
                  DropdownMenuItem(value: 'all', child: Text('All node types')),
                  DropdownMenuItem(
                    value: 'listen',
                    child: Text('Listen nodes'),
                  ),
                  DropdownMenuItem(
                    value: 'interface',
                    child: Text('Interface nodes'),
                  ),
                  DropdownMenuItem(
                    value: 'broadcast',
                    child: Text('Broadcast nodes'),
                  ),
                ],
                onChanged: (value) {
                  setState(() {
                    _nodeType = value ?? 'all';
                    _resetSelectedSetting();
                  });
                },
              ),
              const SizedBox(height: 16),
              DropdownButtonFormField<String>(
                key: ValueKey('setting-$_nodeType-$_selectedKey'),
                initialValue: _selectedKey,
                decoration: const InputDecoration(labelText: 'Setting'),
                items: _settings
                    .map(
                      (entry) => DropdownMenuItem(
                        value: entry.key,
                        child: Text(_settingLabel(entry.key)),
                      ),
                    )
                    .toList(),
                onChanged: setting == null
                    ? null
                    : (value) {
                        final next = _settings.firstWhere(
                          (entry) => entry.key == value,
                        );
                        setState(() {
                          _selectedKey = value;
                          _valueController.text = next.value;
                        });
                      },
              ),
              const SizedBox(height: 16),
              if (setting?.valueType == 'bool')
                SwitchListTile.adaptive(
                  contentPadding: EdgeInsets.zero,
                  title: const Text(
                    'Enabled',
                    style: TextStyle(color: Colors.white),
                  ),
                  value: _valueController.text.toLowerCase() == 'true',
                  onChanged: (value) {
                    setState(() => _valueController.text = value.toString());
                  },
                )
              else
                TextFormField(
                  controller: _valueController,
                  keyboardType: setting == null
                      ? TextInputType.text
                      : _keyboardTypeFor(setting),
                  style: const TextStyle(color: Colors.white),
                  decoration: InputDecoration(
                    labelText: setting == null ? 'Value' : 'New value',
                    filled: true,
                    fillColor: const Color(0xFF0B1423),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(6),
                    ),
                  ),
                ),
              const SizedBox(height: 14),
              Text(
                '$_targetCount connected node${_targetCount == 1 ? '' : 's'} will be considered.',
                style: const TextStyle(color: Color(0xFF86EFAC), fontSize: 12),
              ),
              if (_errorMessage != null) ...[
                const SizedBox(height: 12),
                Text(
                  _errorMessage!,
                  style: const TextStyle(
                    color: Color(0xFFFCA5A5),
                    fontSize: 12,
                  ),
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
          onPressed: setting == null || _isSaving ? null : _save,
          icon: _isSaving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.done_all_outlined),
          label: const Text('Apply'),
        ),
      ],
    );
  }
}

class MediaLibraryTab extends StatefulWidget {
  final AdminApi api;

  const MediaLibraryTab({super.key, required this.api});

  @override
  State<MediaLibraryTab> createState() => _MediaLibraryTabState();
}

class _MediaLibraryTabState extends State<MediaLibraryTab> {
  late final MediaPlayer _player;
  StreamSubscription<void>? _endedSubscription;
  List<MediaAsset> _assets = const [];
  String? _errorMessage;
  String? _playingId;
  bool _isLoading = true;
  bool _isUploading = false;

  @override
  void initState() {
    super.initState();
    _player = createMediaPlayer();
    _endedSubscription = _player.onEnded.listen((_) {
      if (mounted) {
        setState(() => _playingId = null);
      }
    });
    _loadMedia();
  }

  @override
  void dispose() {
    _endedSubscription?.cancel();
    _player.dispose();
    super.dispose();
  }

  Future<void> _loadMedia() async {
    try {
      final assets = await widget.api.fetchMedia();
      if (!mounted) return;
      setState(() {
        _assets = assets;
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
        _errorMessage = 'Unable to reach the media library';
        _isLoading = false;
      });
    }
  }

  Future<void> _upload() async {
    if (_isUploading) return;
    final file = await chooseMp3File();
    if (file == null || !mounted) return;

    setState(() => _isUploading = true);
    try {
      final asset = await widget.api.uploadMedia(file);
      if (!mounted) return;
      await _loadMedia();
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('${asset.name} uploaded')));
    } on AdminApiException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Unable to upload MP3 file')),
      );
    } finally {
      if (mounted) {
        setState(() => _isUploading = false);
      }
    }
  }

  Future<void> _togglePlayback(MediaAsset asset) async {
    if (_playingId == asset.id) {
      _player.pause();
      setState(() => _playingId = null);
      return;
    }

    try {
      await _player.play(widget.api.streamUrl(asset));
      if (mounted) {
        setState(() => _playingId = asset.id);
      }
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Audio playback could not start')),
      );
    }
  }

  Future<void> _delete(MediaAsset asset) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        backgroundColor: const Color(0xFF101A2E),
        title: const Text('Delete MP3', style: TextStyle(color: Colors.white)),
        content: Text(
          'Delete ${asset.name}? This cannot be undone.',
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
      await widget.api.deleteMedia(asset.id);
      if (_playingId == asset.id) {
        _player.pause();
        _playingId = null;
      }
      await _loadMedia();
    } on AdminApiException catch (error) {
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('Unable to delete media')));
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) {
      return const Center(child: CircularProgressIndicator());
    }

    return RefreshIndicator(
      onRefresh: _loadMedia,
      child: CustomScrollView(
        physics: const AlwaysScrollableScrollPhysics(),
        slivers: [
          SliverToBoxAdapter(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(24, 24, 24, 12),
              child: Wrap(
                alignment: WrapAlignment.spaceBetween,
                crossAxisAlignment: WrapCrossAlignment.center,
                runSpacing: 12,
                children: [
                  const Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        'Audio library',
                        style: TextStyle(
                          color: Colors.white,
                          fontSize: 22,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      SizedBox(height: 4),
                      Text(
                        'Stored MP3 files are ready for future routing.',
                        style: TextStyle(color: Color(0xFFA9B7C9)),
                      ),
                    ],
                  ),
                  FilledButton.icon(
                    onPressed: _isUploading ? null : _upload,
                    icon: _isUploading
                        ? const SizedBox(
                            width: 16,
                            height: 16,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(Icons.upload_file_outlined),
                    label: Text(_isUploading ? 'Uploading' : 'Upload MP3'),
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
                title: 'Media library unavailable',
                message: _errorMessage!,
                action: OutlinedButton.icon(
                  onPressed: _loadMedia,
                  icon: const Icon(Icons.refresh_outlined),
                  label: const Text('Retry'),
                ),
              ),
            )
          else if (_assets.isEmpty)
            const SliverFillRemaining(
              hasScrollBody: false,
              child: _AdminEmptyState(
                icon: Icons.audio_file_outlined,
                title: 'No MP3 files yet',
                message:
                    'Upload an MP3 to start building the server audio library.',
              ),
            )
          else
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(24, 12, 24, 32),
              sliver: SliverGrid(
                gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
                  maxCrossAxisExtent: 310,
                  mainAxisExtent: 224,
                  mainAxisSpacing: 12,
                  crossAxisSpacing: 12,
                ),
                delegate: SliverChildBuilderDelegate((context, index) {
                  final asset = _assets[index];
                  return MediaAssetCard(
                    asset: asset,
                    isPlaying: _playingId == asset.id,
                    onPlay: () => _togglePlayback(asset),
                    onDelete: () => _delete(asset),
                  );
                }, childCount: _assets.length),
              ),
            ),
        ],
      ),
    );
  }
}

class MediaAssetCard extends StatelessWidget {
  final MediaAsset asset;
  final bool isPlaying;
  final VoidCallback onPlay;
  final VoidCallback onDelete;

  const MediaAssetCard({
    super.key,
    required this.asset,
    required this.isPlaying,
    required this.onPlay,
    required this.onDelete,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF111D2F),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(
          color: isPlaying
              ? const Color(0xFF2DD4BF)
              : Colors.white.withValues(alpha: 0.08),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 34,
                height: 34,
                decoration: BoxDecoration(
                  color: const Color(0xFF164E63),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: const Icon(Icons.music_note, color: Color(0xFF67E8F9)),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  asset.name,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    color: Colors.white,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              Tooltip(
                message: 'Delete ${asset.name}',
                child: IconButton(
                  onPressed: onDelete,
                  icon: const Icon(Icons.delete_outline),
                  color: const Color(0xFFFCA5A5),
                ),
              ),
            ],
          ),
          const Spacer(),
          _MediaWaveform(seed: _waveformSeed(asset.id)),
          const SizedBox(height: 14),
          Text(
            '${_formatBytes(asset.sizeBytes)}  |  ${_formatUploadTime(asset.uploadedAt)}',
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(color: Color(0xFFA9B7C9), fontSize: 12),
          ),
          const SizedBox(height: 10),
          Row(
            children: [
              Expanded(
                child: Text(
                  isPlaying ? 'Playing now' : 'MP3 audio',
                  style: TextStyle(
                    color: isPlaying
                        ? const Color(0xFF86EFAC)
                        : const Color(0xFFA9B7C9),
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
              Tooltip(
                message: isPlaying ? 'Pause audio' : 'Play audio',
                child: IconButton.filled(
                  onPressed: onPlay,
                  icon: Icon(isPlaying ? Icons.pause : Icons.play_arrow),
                  style: IconButton.styleFrom(
                    backgroundColor: const Color(0xFF2DD4BF),
                    foregroundColor: const Color(0xFF07111B),
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _MediaWaveform extends StatelessWidget {
  final int seed;

  const _MediaWaveform({required this.seed});

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 54,
      width: double.infinity,
      child: CustomPaint(painter: _MediaWaveformPainter(seed)),
    );
  }
}

class _MediaWaveformPainter extends CustomPainter {
  final int seed;

  const _MediaWaveformPainter(this.seed);

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = const Color(0xFF2DD4BF).withValues(alpha: 0.7)
      ..strokeCap = StrokeCap.round;
    const barCount = 28;
    final barWidth = size.width / (barCount * 1.65);
    final gap = barWidth * 0.65;
    var state = seed;

    for (var index = 0; index < barCount; index++) {
      state = (state * 1103515245 + 12345) & 0x7fffffff;
      final factor = 0.25 + (state % 70) / 100;
      final height = size.height * factor;
      final left = index * (barWidth + gap);
      final top = (size.height - height) / 2;
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          Rect.fromLTWH(left, top, barWidth, height),
          Radius.circular(barWidth),
        ),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(covariant _MediaWaveformPainter oldDelegate) {
    return oldDelegate.seed != seed;
  }
}

class _NodeStatusPill extends StatelessWidget {
  final bool active;

  const _NodeStatusPill({required this.active});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: (active ? const Color(0xFF065F46) : const Color(0xFF344155))
            .withValues(alpha: 0.6),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        active ? 'Live' : 'Offline',
        style: const TextStyle(
          color: Colors.white,
          fontSize: 12,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}

class _AdminDetail extends StatelessWidget {
  final String label;
  final String value;

  const _AdminDetail({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Container(
      constraints: const BoxConstraints(maxWidth: 220),
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
      decoration: BoxDecoration(
        color: const Color(0xFF111D2F),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            label,
            style: const TextStyle(color: Colors.white54, fontSize: 10),
          ),
          const SizedBox(height: 2),
          Text(
            value,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(color: Colors.white, fontSize: 12),
          ),
        ],
      ),
    );
  }
}

class _AdminEmptyState extends StatelessWidget {
  final IconData icon;
  final String title;
  final String message;
  final Widget? action;

  const _AdminEmptyState({
    required this.icon,
    required this.title,
    required this.message,
    this.action,
  });

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 36, color: const Color(0xFF67E8F9)),
            const SizedBox(height: 14),
            Text(
              title,
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: Colors.white,
                fontSize: 18,
                fontWeight: FontWeight.w700,
              ),
            ),
            const SizedBox(height: 6),
            Text(
              message,
              textAlign: TextAlign.center,
              style: const TextStyle(color: Color(0xFFA9B7C9)),
            ),
            if (action != null) ...[const SizedBox(height: 16), action!],
          ],
        ),
      ),
    );
  }
}

TextInputType _keyboardTypeFor(NodeConfigEntryViewModel entry) {
  switch (entry.valueType) {
    case 'int':
      return const TextInputType.numberWithOptions(signed: true);
    case 'float':
      return const TextInputType.numberWithOptions(decimal: true, signed: true);
    default:
      return TextInputType.text;
  }
}

String _nodeTypeLabel(String type) {
  switch (type) {
    case 'listen':
      return 'Listen';
    case 'interface':
      return 'Interface';
    case 'broadcast':
      return 'Broadcast';
    case 'all':
      return 'All';
    default:
      return 'Unknown';
  }
}

String _settingLabel(String key) {
  return key.replaceAllMapped(
    RegExp(r'([a-z0-9])([A-Z])'),
    (match) => '${match[1]} ${match[2]}',
  );
}

String _formatBytes(int bytes) {
  if (bytes < 1024) return '$bytes B';
  if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
  return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
}

String _formatUploadTime(String rawDate) {
  final date = DateTime.tryParse(rawDate)?.toLocal();
  if (date == null) return 'Recently uploaded';
  return '${date.month}/${date.day}/${date.year}';
}

int _waveformSeed(String value) {
  var result = 17;
  for (final unit in value.codeUnits) {
    result = (result * 31 + unit) & 0x7fffffff;
  }
  return result;
}

class _MetricBadge extends StatelessWidget {
  final String label;
  final String value;

  const _MetricBadge({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 10),
      decoration: BoxDecoration(
        color: const Color(0xFF111B2B),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(
              color: Colors.white60,
              fontSize: 11,
              letterSpacing: 0.8,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            value,
            style: const TextStyle(
              color: Colors.white,
              fontWeight: FontWeight.w700,
              fontSize: 20,
            ),
          ),
        ],
      ),
    );
  }
}

int _countActive(TopologyResponse topology) {
  final activeListen = topology.listen.where((node) => node.active).length;
  final activeInterface = topology.interfaceNodes
      .where((node) => node.active)
      .length;
  final activeBroadcast = topology.broadcast
      .where((node) => node.active)
      .length;
  return activeListen +
      activeInterface +
      activeBroadcast +
      (topology.server.active ? 1 : 0);
}

class TopologyDiagram extends StatelessWidget {
  final TopologyResponse topology;
  final List<TopologyConnection> connections;

  const TopologyDiagram({
    super.key,
    required this.topology,
    required this.connections,
  });

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        const boxWidth = 170.0;
        const boxHeight = 108.0;
        const rowGap = 18.0;
        const serverWidth = 210.0;
        const serverHeight = 96.0;

        final listenWidth = topology.listen.isEmpty
            ? boxWidth
            : topology.listen.length * boxWidth +
                  (topology.listen.length - 1) * rowGap;
        final interfaceWidth = topology.interfaceNodes.isEmpty
            ? boxWidth
            : topology.interfaceNodes.length * boxWidth +
                  (topology.interfaceNodes.length - 1) * rowGap;
        final broadcastWidth = topology.broadcast.isEmpty
            ? boxWidth
            : topology.broadcast.length * boxWidth +
                  (topology.broadcast.length - 1) * rowGap;

        final listenStart = (constraints.maxWidth - listenWidth) / 2;
        final interfaceStart = (constraints.maxWidth - interfaceWidth) / 2;
        final broadcastStart = (constraints.maxWidth - broadcastWidth) / 2;
        final serverX = (constraints.maxWidth - serverWidth) / 2;

        final listenRowTop = 20.0;
        final interfaceRowTop = 140.0;
        final serverRowTop = 270.0;
        final broadcastRowTop = 410.0;

        return SizedBox(
          width: constraints.maxWidth,
          height: 525,
          child: Stack(
            children: [
              Positioned.fill(
                child: CustomPaint(
                  painter: TopologyPainter(
                    listenNodes: topology.listen,
                    interfaceNodes: topology.interfaceNodes,
                    server: topology.server,
                    broadcastNodes: topology.broadcast,
                    listenRowTop: listenRowTop,
                    interfaceRowTop: interfaceRowTop,
                    serverRowTop: serverRowTop,
                    broadcastRowTop: broadcastRowTop,
                    boxWidth: boxWidth,
                    boxHeight: boxHeight,
                    serverWidth: serverWidth,
                    serverHeight: serverHeight,
                    listenStart: listenStart,
                    interfaceStart: interfaceStart,
                    broadcastStart: broadcastStart,
                    serverX: serverX,
                  ),
                ),
              ),
              Positioned(
                top: listenRowTop,
                left: listenStart,
                child: Wrap(
                  spacing: rowGap,
                  children: topology.listen.isEmpty
                      ? [
                          _NodeCard(
                            node: const PublicNode(
                              peerId: 0,
                              name: 'No listen nodes',
                              active: false,
                              rssi: null,
                              snr: null,
                            ),
                            compact: true,
                          ),
                        ]
                      : topology.listen
                            .map(
                              (node) => _NodeCard(
                                node: node,
                                width: boxWidth,
                                height: boxHeight,
                                showLastKeyUp: true,
                              ),
                            )
                            .toList(),
                ),
              ),
              Positioned(
                top: interfaceRowTop,
                left: interfaceStart,
                child: Wrap(
                  spacing: rowGap,
                  children: topology.interfaceNodes.isEmpty
                      ? [
                          _NodeCard(
                            node: const PublicNode(
                              peerId: 0,
                              name: 'No interface nodes',
                              active: false,
                              rssi: null,
                              snr: null,
                            ),
                            compact: true,
                          ),
                        ]
                      : topology.interfaceNodes
                            .map(
                              (node) => _NodeCard(
                                node: node,
                                width: boxWidth,
                                height: boxHeight,
                              ),
                            )
                            .toList(),
                ),
              ),
              Positioned(
                top: serverRowTop,
                left: serverX,
                child: _NodeCard(
                  node: topology.server,
                  width: serverWidth,
                  height: serverHeight,
                  isServer: true,
                ),
              ),
              Positioned(
                top: broadcastRowTop,
                left: broadcastStart,
                child: Wrap(
                  spacing: rowGap,
                  children: topology.broadcast.isEmpty
                      ? [
                          _NodeCard(
                            node: const PublicNode(
                              peerId: 0,
                              name: 'No broadcast nodes',
                              active: false,
                              rssi: null,
                              snr: null,
                            ),
                            compact: true,
                          ),
                        ]
                      : topology.broadcast
                            .map(
                              (node) => _NodeCard(
                                node: node,
                                width: boxWidth,
                                height: boxHeight,
                              ),
                            )
                            .toList(),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

class TopologyPainter extends CustomPainter {
  final List<PublicNode> listenNodes;
  final List<PublicNode> interfaceNodes;
  final PublicNode server;
  final List<PublicNode> broadcastNodes;
  final double listenRowTop;
  final double interfaceRowTop;
  final double serverRowTop;
  final double broadcastRowTop;
  final double boxWidth;
  final double boxHeight;
  final double serverWidth;
  final double serverHeight;
  final double listenStart;
  final double interfaceStart;
  final double broadcastStart;
  final double serverX;

  TopologyPainter({
    required this.listenNodes,
    required this.interfaceNodes,
    required this.server,
    required this.broadcastNodes,
    required this.listenRowTop,
    required this.interfaceRowTop,
    required this.serverRowTop,
    required this.broadcastRowTop,
    required this.boxWidth,
    required this.boxHeight,
    required this.serverWidth,
    required this.serverHeight,
    required this.listenStart,
    required this.interfaceStart,
    required this.broadcastStart,
    required this.serverX,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final mutedPaint = Paint()
      ..color = const Color(0xFF68768B)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 3
      ..strokeCap = StrokeCap.round;

    final activePaint = Paint()
      ..color = const Color(0xFFFF4D4D)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 4
      ..strokeCap = StrokeCap.round
      ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 2);

    for (var i = 0; i < listenNodes.length; i++) {
      final node = listenNodes[i];
      final start = Offset(
        listenStart + (i * (boxWidth + 18)) + (boxWidth / 2),
        listenRowTop + boxHeight,
      );
      final end = Offset(serverX + (serverWidth / 2), serverRowTop);
      final path = Path();
      path.moveTo(start.dx, start.dy);
      path.cubicTo(
        start.dx,
        start.dy + 46,
        end.dx,
        end.dy - 46,
        end.dx,
        end.dy,
      );
      canvas.drawPath(path, node.active ? activePaint : mutedPaint);
    }

    for (var i = 0; i < interfaceNodes.length; i++) {
      final node = interfaceNodes[i];
      final start = Offset(
        interfaceStart + (i * (boxWidth + 18)) + (boxWidth / 2),
        interfaceRowTop + boxHeight,
      );
      final end = Offset(serverX + (serverWidth / 2), serverRowTop);
      final path = Path();
      path.moveTo(start.dx, start.dy);
      path.cubicTo(
        start.dx,
        start.dy + 36,
        end.dx,
        end.dy - 36,
        end.dx,
        end.dy,
      );
      canvas.drawPath(path, node.active ? activePaint : mutedPaint);
    }

    for (var i = 0; i < broadcastNodes.length; i++) {
      final node = broadcastNodes[i];
      final start = Offset(
        serverX + (serverWidth / 2),
        serverRowTop + serverHeight,
      );
      final end = Offset(
        broadcastStart + (i * (boxWidth + 18)) + (boxWidth / 2),
        broadcastRowTop,
      );
      final path = Path();
      path.moveTo(start.dx, start.dy);
      path.cubicTo(
        start.dx,
        start.dy + 44,
        end.dx,
        end.dy - 44,
        end.dx,
        end.dy,
      );
      canvas.drawPath(path, node.active ? activePaint : mutedPaint);
    }
  }

  @override
  bool shouldRepaint(covariant TopologyPainter oldDelegate) => true;
}

class _NodeCard extends StatelessWidget {
  final PublicNode node;
  final double width;
  final double height;
  final bool isServer;
  final bool compact;
  final bool showLastKeyUp;

  const _NodeCard({
    required this.node,
    this.width = 170,
    this.height = 84,
    this.isServer = false,
    this.compact = false,
    this.showLastKeyUp = false,
  });

  @override
  Widget build(BuildContext context) {
    final active = node.active;
    final borderColor = active
        ? const Color(0xFFFF4D4D)
        : Colors.white.withValues(alpha: 0.12);

    return Container(
      width: width,
      height: height,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: isServer ? const Color(0xFF171F30) : const Color(0xFF101B2A),
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: borderColor, width: active ? 1.4 : 1),
        boxShadow: [
          BoxShadow(
            color: (active ? const Color(0xFFFF4D4D) : const Color(0xFF3B4658))
                .withValues(alpha: 0.25),
            blurRadius: active ? 18 : 10,
            spreadRadius: active ? 0.5 : 0,
          ),
        ],
      ),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 10,
                height: 10,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: active
                      ? const Color(0xFFFF4D4D)
                      : const Color(0xFF69778B),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  node.name,
                  overflow: TextOverflow.ellipsis,
                  maxLines: 1,
                  style: const TextStyle(
                    color: Colors.white,
                    fontWeight: FontWeight.w600,
                    fontSize: 15,
                  ),
                ),
              ),
            ],
          ),
          if (!isServer && (node.rssi != null || node.snr != null)) ...[
            const SizedBox(height: 12),
            FittedBox(
              alignment: Alignment.centerLeft,
              fit: BoxFit.scaleDown,
              child: Text(
                'RSSI ${node.rssi?.toStringAsFixed(0) ?? 'n/a'} dB | SNR ${node.snr?.toStringAsFixed(1) ?? 'n/a'} dB',
                style: const TextStyle(
                  color: Color(0xFFA9B7C9),
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ),
          ] else if (isServer) ...[
            const SizedBox(height: 8),
            const Text(
              'System core',
              style: TextStyle(
                color: Color(0xFF9DB6C7),
                fontSize: 11,
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
          if (showLastKeyUp) ...[
            const SizedBox(height: 4),
            Text(
              node.lastKeyUpAgo.isEmpty
                  ? 'Last keyup: never'
                  : 'Last keyup: ${node.lastKeyUpAgo} ago',
              style: const TextStyle(
                color: Color(0xFF9DB6C7),
                fontSize: 11,
                fontWeight: FontWeight.w500,
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class TopologyResponse {
  final List<PublicNode> listen;
  final List<PublicNode> interfaceNodes;
  final PublicNode server;
  final List<PublicNode> broadcast;

  const TopologyResponse({
    required this.listen,
    required this.interfaceNodes,
    required this.server,
    required this.broadcast,
  });

  factory TopologyResponse.empty() {
    return TopologyResponse(
      listen: const [],
      interfaceNodes: const [],
      server: const PublicNode(
        peerId: 0,
        name: 'Server',
        active: false,
        rssi: null,
        snr: null,
      ),
      broadcast: const [],
    );
  }

  factory TopologyResponse.fromJson(Map<String, dynamic> json) {
    final listenRaw = json['listen'] as List? ?? const [];
    final interfaceRaw = json['interface'] as List? ?? const [];
    final broadcastRaw = json['broadcast'] as List? ?? const [];
    final serverRaw = json['server'] as Map<String, dynamic>? ?? const {};

    return TopologyResponse(
      listen: listenRaw
          .map((entry) => PublicNode.fromJson(entry as Map<String, dynamic>))
          .toList(),
      interfaceNodes: interfaceRaw
          .map((entry) => PublicNode.fromJson(entry as Map<String, dynamic>))
          .toList(),
      server: PublicNode.fromJson(serverRaw),
      broadcast: broadcastRaw
          .map((entry) => PublicNode.fromJson(entry as Map<String, dynamic>))
          .toList(),
    );
  }
}

class PublicNode {
  final int peerId;
  final String name;
  final bool active;
  final double? rssi;
  final double? snr;
  final String lastKeyUpAgo;

  const PublicNode({
    required this.peerId,
    required this.name,
    required this.active,
    required this.rssi,
    required this.snr,
    this.lastKeyUpAgo = '',
  });

  factory PublicNode.fromJson(Map<String, dynamic> json) {
    final peerIdValue = (json['peerId'] as num?)?.toInt() ?? 0;
    final rawRssi = json['rssi'];
    final rawSnr = json['snr'];
    double? parsedRssi;
    double? parsedSnr;
    if (rawRssi != null) {
      parsedRssi = (rawRssi as num).toDouble();
    }
    if (rawSnr != null) {
      parsedSnr = (rawSnr as num).toDouble();
    }

    final requestedName = (json['name'] as String? ?? 'Peer $peerIdValue')
        .trim();

    return PublicNode(
      peerId: peerIdValue,
      name: requestedName.isEmpty ? 'Peer $peerIdValue' : requestedName,
      active: json['active'] as bool? ?? false,
      rssi: parsedRssi,
      snr: parsedSnr,
      lastKeyUpAgo: json['lastKeyUpAgo'] as String? ?? '',
    );
  }
}

class TopologyConnection {
  final String id;
  final String fromNodeId;
  final String toNodeId;
  final String color;
  final double width;

  const TopologyConnection({
    required this.id,
    required this.fromNodeId,
    required this.toNodeId,
    required this.color,
    required this.width,
  });

  factory TopologyConnection.fromJson(Map<String, dynamic> json) {
    return TopologyConnection(
      id: json['id'] as String? ?? '',
      fromNodeId: json['fromNodeId'] as String? ?? '',
      toNodeId: json['toNodeId'] as String? ?? '',
      color: json['color'] as String? ?? '#6B7280',
      width: (json['width'] as num?)?.toDouble() ?? 2,
    );
  }
}

class NodeConfigViewModel {
  final int peerId;
  final String name;
  final String nodeType;
  final String remoteAddr;
  final String lastHeartbeat;
  final String lastHeartbeatAgo;
  final bool active;
  final double? rssi;
  final double? snr;
  final List<NodeConfigEntryViewModel> config;

  const NodeConfigViewModel({
    required this.peerId,
    required this.name,
    required this.nodeType,
    required this.remoteAddr,
    required this.lastHeartbeat,
    required this.lastHeartbeatAgo,
    required this.active,
    required this.rssi,
    required this.snr,
    required this.config,
  });

  factory NodeConfigViewModel.fromJson(Map<String, dynamic> json) {
    final rawConfig = json['config'] as List? ?? const [];
    return NodeConfigViewModel(
      peerId: (json['peerId'] as num?)?.toInt() ?? 0,
      name: json['name'] as String? ?? 'Peer ${json['peerId'] ?? 0}',
      nodeType: json['nodeType'] as String? ?? 'unknown',
      remoteAddr: json['remoteAddr'] as String? ?? 'unknown',
      lastHeartbeat: json['lastHeartbeat'] as String? ?? '',
      lastHeartbeatAgo: json['lastHeartbeatAgo'] as String? ?? '',
      active: json['active'] as bool? ?? false,
      rssi: (json['rssi'] is num) ? (json['rssi'] as num).toDouble() : null,
      snr: (json['snr'] is num) ? (json['snr'] as num).toDouble() : null,
      config: rawConfig
          .map(
            (entry) => NodeConfigEntryViewModel.fromJson(
              entry as Map<String, dynamic>,
            ),
          )
          .toList(),
    );
  }
}

class NodeConfigEntryViewModel {
  final String key;
  final String type;
  final String value;
  final String valueType;

  const NodeConfigEntryViewModel({
    required this.key,
    required this.type,
    required this.value,
    required this.valueType,
  });

  factory NodeConfigEntryViewModel.fromJson(Map<String, dynamic> json) {
    return NodeConfigEntryViewModel(
      key: json['key'] as String? ?? '',
      type: json['type'] as String? ?? '',
      value: json['value'] as String? ?? '',
      valueType: json['valueType'] as String? ?? 'string',
    );
  }
}
