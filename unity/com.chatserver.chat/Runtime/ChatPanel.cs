// ChatPanel is a ready-made in-game chat box for a ChatClient. It builds its
// own UI at start (no prefab or assets needed), so adding the component is
// enough. Restyle it by changing the fields, or use it as a starting point
// for your own UI.
//
// Controls (rebindable below):
//   Enter or T   open the box and type     Enter   send
//   Esc          close without sending     Tab     next room, while typing
//
// While the box is open, IsTyping is true and TypingChanged fires: stop
// your game reading movement keys then, or players walk while they type.
using System;
using System.Collections.Generic;
using UnityEngine;
using UnityEngine.EventSystems;
using UnityEngine.UI;

namespace ChatServer.Chat
{
    [RequireComponent(typeof(ChatClient))]
    public class ChatPanel : MonoBehaviour
    {
        [Header("Look")]
        [Tooltip("Leave empty to use Unity's built-in font.")]
        public Font font;
        public int fontSize = 18;
        public Vector2 size = new Vector2(460, 260);
        [Tooltip("From the bottom-left corner of the screen, in reference pixels (1920x1080).")]
        public Vector2 offset = new Vector2(24, 120);
        public Color nameColor = new Color(0.55f, 0.83f, 1f);
        public Color systemColor = new Color(1f, 0.55f, 0.45f);
        [Range(0, 1)] public float backgroundOpacity = 0.55f;

        [Header("Behaviour")]
        [Tooltip("Seconds a message stays visible while the box is closed. 0 = forever.")]
        public float fadeAfter = 10f;
        public int maxLines = 100;

        [Header("Keys (old Input Manager)")]
        public KeyCode openKey = KeyCode.T;
        public KeyCode sendKey = KeyCode.Return;
        public KeyCode closeKey = KeyCode.Escape;
        public KeyCode nextRoomKey = KeyCode.Tab;

        /// <summary>True while the player is typing in the box.</summary>
        public bool IsTyping { get; private set; }

        /// <summary>Fires when typing starts (true) or stops (false).</summary>
        public event Action<bool> TypingChanged;

        /// <summary>The room messages are sent to: the first one joined, unless changed.</summary>
        public string ActiveRoomId { get; set; }

        class Line
        {
            public GameObject go;
            public CanvasGroup group;
            public float shownAt;
        }

        ChatClient _chat;
        CanvasGroup _background;
        RectTransform _content;
        ScrollRect _scroll;
        InputField _input;
        Text _placeholder;
        readonly List<Line> _lines = new List<Line>();
        readonly HashSet<string> _seen = new HashSet<string>();     // message IDs already shown
        readonly Queue<string> _seenOrder = new Queue<string>();     // to forget old IDs
        readonly List<string> _roomOrder = new List<string>();

        void Awake()
        {
            _chat = GetComponent<ChatClient>();
            if (font == null)
            {
#if UNITY_2022_2_OR_NEWER
                font = Resources.GetBuiltinResource<Font>("LegacyRuntime.ttf");
#else
                font = Resources.GetBuiltinResource<Font>("Arial.ttf");
#endif
            }
            BuildUI();
        }

        void OnEnable()
        {
            _chat.Joined += OnJoined;
            _chat.MessageReceived += OnMessage;
            _chat.Left += OnLeft;
            _chat.Error += OnError;
            _chat.Disconnected += OnDisconnected;
            _chat.AuthFailed += OnAuthFailed;
        }

        void OnDisable()
        {
            _chat.Joined -= OnJoined;
            _chat.MessageReceived -= OnMessage;
            _chat.Left -= OnLeft;
            _chat.Error -= OnError;
            _chat.Disconnected -= OnDisconnected;
            _chat.AuthFailed -= OnAuthFailed;
            SetTyping(false);
        }

        // ---- events from the client ----

        void OnJoined(ChatRoom room, ChatMessage[] history)
        {
            if (!_roomOrder.Contains(room.id))
                _roomOrder.Add(room.id);
            if (string.IsNullOrEmpty(ActiveRoomId))
                ActiveRoomId = room.id;
            foreach (var m in history)
                AddMessage(m);
            UpdatePlaceholder();
        }

        void OnMessage(ChatMessage m) => AddMessage(m);

        void OnLeft(string roomId, string reason)
        {
            _roomOrder.Remove(roomId);
            if (ActiveRoomId == roomId)
                ActiveRoomId = _roomOrder.Count > 0 ? _roomOrder[0] : null;
            UpdatePlaceholder();
        }

        void OnError(ChatError e)
        {
            switch (e.Code)
            {
                case "rate_limited":
                    AddSystem("You're sending messages too fast.");
                    break;
                case "forbidden":
                    AddSystem("You can't chat in that room.");
                    break;
                case "not_connected":
                    AddSystem("Not connected to chat.");
                    break;
                default:
                    AddSystem(e.Message);
                    break;
            }
        }

        void OnDisconnected(string reason, bool willRetry) =>
            AddSystem(willRetry ? "Chat disconnected. Reconnecting..." : "Chat disconnected.");

        void OnAuthFailed(string reason) => AddSystem("Couldn't sign in to chat.");

        // ---- input ----

        void Update()
        {
            if (!IsTyping)
            {
                if (KeyDown(openKey) || KeyDown(sendKey))
                    Open();
            }
            else
            {
                if (KeyDown(closeKey))
                    Close();
                else if (KeyDown(sendKey))
                    Submit();
                else if (KeyDown(nextRoomKey))
                    NextRoom();
            }
            Fade();
        }

        /// <summary>Opens the box for typing.</summary>
        public void Open()
        {
            SetTyping(true);
            _input.text = "";
            _input.ActivateInputField();
            _input.Select();
        }

        /// <summary>Closes the box without sending.</summary>
        public void Close()
        {
            _input.DeactivateInputField();
            if (EventSystem.current != null)
                EventSystem.current.SetSelectedGameObject(null);
            SetTyping(false);
        }

        void Submit()
        {
            var text = _input.text.Trim();
            if (text.Length > 0)
            {
                if (string.IsNullOrEmpty(ActiveRoomId))
                    AddSystem("Not in a chat room yet.");
                else
                    _chat.Send(ActiveRoomId, text);
            }
            Close();
        }

        void NextRoom()
        {
            if (_roomOrder.Count < 2)
                return;
            var i = _roomOrder.IndexOf(ActiveRoomId);
            ActiveRoomId = _roomOrder[(i + 1) % _roomOrder.Count];
            UpdatePlaceholder();
            _input.ActivateInputField(); // Tab may have moved the focus
        }

        void SetTyping(bool typing)
        {
            if (IsTyping == typing)
                return;
            IsTyping = typing;
            TypingChanged?.Invoke(typing);
        }

        static bool KeyDown(KeyCode key)
        {
#if ENABLE_LEGACY_INPUT_MANAGER
            return Input.GetKeyDown(key);
#elif CHATSERVER_INPUT_SYSTEM
            return InputSystemKeys.WasPressed(key);
#else
            return false; // no input backend: call Open() and Close() from your own input code
#endif
        }

        // ---- lines ----

        void AddMessage(ChatMessage m)
        {
            if (m == null || string.IsNullOrEmpty(m.id) || !_seen.Add(m.id))
                return; // shown already (history and the live feed can overlap)
            _seenOrder.Enqueue(m.id);
            while (_seenOrder.Count > maxLines * 4)
                _seen.Remove(_seenOrder.Dequeue());

            var prefix = "";
            if (_roomOrder.Count > 1 && _chat.Rooms.TryGetValue(m.room, out var room))
                prefix = $"[{Escape(room.name)}] ";
            var name = string.IsNullOrEmpty(m.author_name) ? m.author : m.author_name;
            AddLine($"{prefix}<b><color=#{ColorUtility.ToHtmlStringRGB(nameColor)}>{Escape(name)}</color></b>: {Escape(m.text)}");
        }

        void AddSystem(string text) =>
            AddLine($"<i><color=#{ColorUtility.ToHtmlStringRGB(systemColor)}>{Escape(text)}</color></i>");

        // Stops chat text from being read as rich-text tags ("<b>", "<size=200>").
        static string Escape(string s) => string.IsNullOrEmpty(s) ? "" : s.Replace("<", "<​");

        void AddLine(string richText)
        {
            var go = new GameObject("Line", typeof(RectTransform), typeof(CanvasGroup));
            go.transform.SetParent(_content, false);
            var text = go.AddComponent<Text>();
            text.font = font;
            text.fontSize = fontSize;
            text.supportRichText = true;
            text.color = Color.white;
            text.horizontalOverflow = HorizontalWrapMode.Wrap;
            text.verticalOverflow = VerticalWrapMode.Overflow;
            text.text = richText;
            text.raycastTarget = false; // lines never catch clicks meant for the game
            var shadow = go.AddComponent<Shadow>();
            shadow.effectColor = new Color(0, 0, 0, 0.8f);
            // The layout group sizes each line to its text: no ContentSizeFitter here.

            _lines.Add(new Line { go = go, group = go.GetComponent<CanvasGroup>(), shownAt = Time.unscaledTime });
            while (_lines.Count > maxLines)
            {
                Destroy(_lines[0].go);
                _lines.RemoveAt(0);
            }
            Canvas.ForceUpdateCanvases();
            _scroll.verticalNormalizedPosition = 0; // keep the newest in view
        }

        void Fade()
        {
            _background.alpha = IsTyping ? backgroundOpacity : 0;
            _background.blocksRaycasts = IsTyping; // never block clicks meant for the game
            foreach (var line in _lines)
            {
                if (IsTyping || fadeAfter <= 0)
                {
                    line.group.alpha = 1;
                    continue;
                }
                var age = Time.unscaledTime - line.shownAt;
                line.group.alpha = Mathf.Clamp01(1 - (age - fadeAfter)); // fades out over 1s
            }
        }

        void UpdatePlaceholder()
        {
            if (_placeholder == null)
                return;
            _placeholder.text = !string.IsNullOrEmpty(ActiveRoomId) && _chat.Rooms.TryGetValue(ActiveRoomId, out var room)
                ? $"Say something in {room.name}..." + (_roomOrder.Count > 1 ? " (Tab: next room)" : "")
                : "Say something...";
        }

        // ---- building the UI ----

        void BuildUI()
        {
            EnsureEventSystem();

            var canvasGo = new GameObject("Chat Canvas", typeof(Canvas), typeof(CanvasScaler), typeof(GraphicRaycaster));
            canvasGo.transform.SetParent(transform, false);
            var canvas = canvasGo.GetComponent<Canvas>();
            canvas.renderMode = RenderMode.ScreenSpaceOverlay;
            canvas.sortingOrder = 100; // above the game's own UI
            var scaler = canvasGo.GetComponent<CanvasScaler>();
            scaler.uiScaleMode = CanvasScaler.ScaleMode.ScaleWithScreenSize;
            scaler.referenceResolution = new Vector2(1920, 1080);
            scaler.matchWidthOrHeight = 0.5f;

            // The panel: bottom-left, with a background that shows only while typing.
            var panel = NewRect("Chat Panel", canvasGo.transform);
            panel.anchorMin = panel.anchorMax = panel.pivot = Vector2.zero;
            panel.anchoredPosition = offset;
            panel.sizeDelta = size;
            // The background is its own layer, so hiding it leaves the lines visible.
            var bgRect = NewRect("Background", panel);
            bgRect.anchorMin = Vector2.zero;
            bgRect.anchorMax = Vector2.one;
            bgRect.sizeDelta = Vector2.zero;
            bgRect.gameObject.AddComponent<Image>().color = Color.black; // also what the mouse wheel scrolls over
            _background = bgRect.gameObject.AddComponent<CanvasGroup>();
            _background.alpha = 0;
            _background.blocksRaycasts = false;

            // The input box along the bottom.
            const float inputHeight = 34;
            var inputRect = NewRect("Input", canvasGo.transform);
            inputRect.anchorMin = inputRect.anchorMax = inputRect.pivot = Vector2.zero;
            inputRect.anchoredPosition = offset - new Vector2(0, inputHeight + 4);
            inputRect.sizeDelta = new Vector2(size.x, inputHeight);
            var inputBg = inputRect.gameObject.AddComponent<Image>();
            inputBg.color = new Color(0, 0, 0, 0.7f);
            _input = inputRect.gameObject.AddComponent<InputField>();
            _input.lineType = InputField.LineType.SingleLine;
            _input.characterLimit = 500; // the server's limit
            var inputText = NewText("Text", inputRect, Color.white);
            _input.textComponent = inputText;
            _placeholder = NewText("Placeholder", inputRect, new Color(1, 1, 1, 0.45f));
            _placeholder.fontStyle = FontStyle.Italic;
            _input.placeholder = _placeholder;
            UpdatePlaceholder();
            var inputGroup = inputRect.gameObject.AddComponent<CanvasGroup>();
            TypingChanged += typing =>
            {
                inputGroup.alpha = typing ? 1 : 0;
                inputGroup.interactable = typing;
                inputGroup.blocksRaycasts = typing;
            };
            inputGroup.alpha = 0;
            inputGroup.interactable = false;
            inputGroup.blocksRaycasts = false;

            // The scrolling list of lines inside the panel.
            _scroll = panel.gameObject.AddComponent<ScrollRect>();
            _scroll.horizontal = false;
            _scroll.movementType = ScrollRect.MovementType.Clamped;
            _scroll.scrollSensitivity = 20;
            var viewport = NewRect("Viewport", panel);
            viewport.anchorMin = Vector2.zero;
            viewport.anchorMax = Vector2.one;
            viewport.offsetMin = new Vector2(8, 6);
            viewport.offsetMax = new Vector2(-8, -6);
            viewport.gameObject.AddComponent<RectMask2D>();
            _content = NewRect("Content", viewport);
            _content.anchorMin = new Vector2(0, 0);
            _content.anchorMax = new Vector2(1, 0);
            _content.pivot = new Vector2(0.5f, 0);
            _content.sizeDelta = Vector2.zero;
            var layout = _content.gameObject.AddComponent<VerticalLayoutGroup>();
            layout.childAlignment = TextAnchor.LowerLeft;
            layout.childControlWidth = true;
            layout.childControlHeight = true;
            layout.childForceExpandWidth = true;
            layout.childForceExpandHeight = false;
            layout.spacing = 2;
            _content.gameObject.AddComponent<ContentSizeFitter>().verticalFit = ContentSizeFitter.FitMode.PreferredSize;
            _scroll.viewport = viewport;
            _scroll.content = _content;
        }

        static RectTransform NewRect(string name, Transform parent)
        {
            var go = new GameObject(name, typeof(RectTransform));
            go.transform.SetParent(parent, false);
            return (RectTransform)go.transform;
        }

        Text NewText(string name, RectTransform parent, Color color)
        {
            var rect = NewRect(name, parent);
            rect.anchorMin = Vector2.zero;
            rect.anchorMax = Vector2.one;
            rect.offsetMin = new Vector2(10, 4);
            rect.offsetMax = new Vector2(-10, -4);
            var text = rect.gameObject.AddComponent<Text>();
            text.font = font;
            text.fontSize = fontSize;
            text.color = color;
            text.alignment = TextAnchor.MiddleLeft;
            text.supportRichText = false; // what the player types is shown as typed
            return text;
        }

        // The InputField needs an EventSystem to receive typing. Add one if
        // the scene has none, with the input module the project uses.
        static void EnsureEventSystem()
        {
            if (EventSystem.current != null || FindAnyEventSystem() != null)
                return;
            var go = new GameObject("EventSystem", typeof(EventSystem));
#if CHATSERVER_INPUT_SYSTEM && !ENABLE_LEGACY_INPUT_MANAGER
            go.AddComponent<UnityEngine.InputSystem.UI.InputSystemUIInputModule>();
#else
            go.AddComponent<StandaloneInputModule>();
#endif
        }

        static EventSystem FindAnyEventSystem()
        {
#if UNITY_2023_1_OR_NEWER
            return FindAnyObjectByType<EventSystem>();
#else
            return FindObjectOfType<EventSystem>();
#endif
        }
    }

#if CHATSERVER_INPUT_SYSTEM && !ENABLE_LEGACY_INPUT_MANAGER
    // Maps the panel's KeyCodes to the new Input System, for projects that
    // use only it.
    static class InputSystemKeys
    {
        public static bool WasPressed(KeyCode key)
        {
            var keyboard = UnityEngine.InputSystem.Keyboard.current;
            if (keyboard == null)
                return false;
            switch (key)
            {
                case KeyCode.Return: return keyboard.enterKey.wasPressedThisFrame || keyboard.numpadEnterKey.wasPressedThisFrame;
                case KeyCode.Escape: return keyboard.escapeKey.wasPressedThisFrame;
                case KeyCode.Tab: return keyboard.tabKey.wasPressedThisFrame;
                case KeyCode.Slash: return keyboard.slashKey.wasPressedThisFrame;
                default:
                    if (key >= KeyCode.A && key <= KeyCode.Z)
                        return keyboard[UnityEngine.InputSystem.Key.A + (key - KeyCode.A)].wasPressedThisFrame;
                    return false;
            }
        }
    }
#endif
}
