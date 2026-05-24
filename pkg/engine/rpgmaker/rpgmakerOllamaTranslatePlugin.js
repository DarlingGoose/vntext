/*:
 * @target MV MZ
 * @plugindesc Translates RPG Maker text through a local Ollama server.
 * @author vntext
 *
 * @param Enabled
 * @type boolean
 * @default true
 *
 * @param Endpoint
 * @type string
 * @default http://127.0.0.1:11434/api/generate
 *
 * @param Model
 * @type string
 * @default llama3.1
 *
 * @param Curl Path
 * @type string
 * @default curl
 *
 * @param Source Language
 * @type string
 * @default Japanese
 *
 * @param Target Language
 * @type string
 * @default English
 *
 * @param Max Dialogue Characters
 * @type number
 * @default 140
 *
 * @param Max Choice Characters
 * @type number
 * @default 48
 *
 * @param Max Window Text Characters
 * @type number
 * @default 64
 *
 * @param Minimum Confidence
 * @type number
 * @default 0.55
 *
 * @param Translate Dialogue
 * @type boolean
 * @default true
 *
 * @param Translate Choices
 * @type boolean
 * @default true
 *
 * @param Translate Window Text
 * @type boolean
 * @default true
 *
 * @param Display Mode
 * @type select
 * @option Replace text in game
 * @value replace
 * @option Popup / info icon
 * @value popup
 * @default replace
 *
 * @param Show Menu Command
 * @type boolean
 * @default true
 *
 * @param Menu Command Name
 * @type string
 * @default Ollama Translation
 *
 * @help
 * Requires Ollama to be running locally, for example:
 *   ollama serve
 *   ollama pull llama3.1
 *
 * Display Mode controls whether translated text replaces the original text
 * before it is drawn, or whether the original text stays in-game and the
 * translation is shown through an on-screen info button/message-window click.
 * Repeated strings are cached for the current game session.
 */
(function() {
    "use strict";

    const PLUGIN_NAME = "WGLOllamaTranslate";
    const params = (typeof PluginManager !== "undefined" && PluginManager.parameters) ?
        PluginManager.parameters(PLUGIN_NAME) : {};

    function param(name, fallback) {
        const value = params[name];
        if (value === undefined || value === null || String(value).trim() === "") {
            return fallback;
        }
        return String(value);
    }

    function boolParam(name, fallback) {
        const value = param(name, fallback ? "true" : "false").toLowerCase();
        return value === "true" || value === "1" || value === "yes" || value === "on";
    }

    function numberParam(name, fallback) {
        const value = Number(param(name, String(fallback)));
        return Number.isFinite(value) && value > 0 ? value : fallback;
    }

    const config = {
        enabled: boolParam("Enabled", true),
        endpoint: param("Endpoint", "http://127.0.0.1:11434/api/generate"),
        model: param("Model", "llama3.1"),
        curlPath: param("Curl Path", "curl"),
        sourceLanguage: param("Source Language", "Japanese"),
        targetLanguage: param("Target Language", "English"),
        maxDialogueCharacters: numberParam("Max Dialogue Characters", 140),
        maxChoiceCharacters: numberParam("Max Choice Characters", 48),
        maxWindowTextCharacters: numberParam("Max Window Text Characters", 64),
        minimumConfidence: numberParam("Minimum Confidence", 0.55),
        translateDialogue: boolParam("Translate Dialogue", true),
        translateChoices: boolParam("Translate Choices", true),
        translateWindowText: boolParam("Translate Window Text", true),
        displayMode: param("Display Mode", "replace"),
        showMenuCommand: boolParam("Show Menu Command", true),
        menuCommandName: param("Menu Command Name", "Ollama Translation")
    };

    const storageKey = "WGLOllamaTranslate.config";
    const cache = {};
    const cacheKeys = [];
    const maxCacheEntries = 1000;
    let lastStatus = "not tested";
    let lastError = "";
    let popupButton = null;
    let popupPanel = null;
    let latestPopupEntries = [];

    function loadStoredConfig() {
        try {
            if (typeof localStorage === "undefined") {
                return;
            }
            const raw = localStorage.getItem(storageKey);
            if (!raw) {
                return;
            }
            const stored = JSON.parse(raw);
            if (!stored || typeof stored !== "object") {
                return;
            }
            Object.keys(config).forEach(function(key) {
                if (Object.prototype.hasOwnProperty.call(stored, key)) {
                    config[key] = stored[key];
                }
            });
        } catch (error) {
            console.warn("WGLOllamaTranslate config load failed:", error);
        }
    }

    function saveStoredConfig() {
        try {
            if (typeof localStorage === "undefined") {
                return;
            }
            localStorage.setItem(storageKey, JSON.stringify(config));
        } catch (error) {
            console.warn("WGLOllamaTranslate config save failed:", error);
        }
    }

    function boolPrompt(label, currentValue) {
        if (typeof window === "undefined" || typeof window.confirm !== "function") {
            return currentValue;
        }
        return window.confirm(label + "\n\nOK = enabled, Cancel = disabled");
    }

    function stringPrompt(label, currentValue) {
        if (typeof window === "undefined" || typeof window.prompt !== "function") {
            return currentValue;
        }
        const value = window.prompt(label, String(currentValue || ""));
        if (value === null) {
            return currentValue;
        }
        return String(value).trim() || currentValue;
    }

    function numberPrompt(label, currentValue) {
        const value = Number(stringPrompt(label, currentValue));
        return Number.isFinite(value) && value > 0 ? Math.floor(value) : currentValue;
    }

    function displayModePrompt(currentValue) {
        const value = stringPrompt("Display mode: replace or popup", currentValue);
        const normalized = normalizeDisplayMode(value);
        return normalized || currentValue;
    }

    function openConfigPrompts() {
        config.enabled = boolPrompt("Enable Ollama translation?", config.enabled);
        config.endpoint = stringPrompt("Ollama endpoint", config.endpoint);
        config.model = stringPrompt("Ollama model", config.model);
        config.curlPath = stringPrompt("curl path", config.curlPath);
        config.sourceLanguage = stringPrompt("Source language", config.sourceLanguage);
        config.targetLanguage = stringPrompt("Target language", config.targetLanguage);
        config.maxDialogueCharacters = numberPrompt("Max dialogue characters", config.maxDialogueCharacters);
        config.maxChoiceCharacters = numberPrompt("Max choice characters", config.maxChoiceCharacters);
        config.maxWindowTextCharacters = numberPrompt("Max window/menu text characters", config.maxWindowTextCharacters);
        config.minimumConfidence = numberPrompt("Minimum confidence from 0 to 1", config.minimumConfidence);
        config.translateDialogue = boolPrompt("Translate dialogue?", config.translateDialogue);
        config.translateChoices = boolPrompt("Translate choices?", config.translateChoices);
        config.translateWindowText = boolPrompt("Translate window/menu text?", config.translateWindowText);
        config.displayMode = displayModePrompt(config.displayMode);
        saveStoredConfig();
        clearCache();
        refreshPopupButton();
        syncConfigManager();
        if (typeof ConfigManager !== "undefined" && typeof ConfigManager.save === "function") {
            ConfigManager.save();
        }
        showStatus(testConnection());
    }

    function clearCache() {
        Object.keys(cache).forEach(function(key) {
            delete cache[key];
        });
        cacheKeys.length = 0;
    }

    loadStoredConfig();

    function normalizeDisplayMode(value) {
        const normalized = String(value || "").trim().toLowerCase();
        if (normalized === "popup" || normalized === "overlay" || normalized === "info") {
            return "popup";
        }
        if (normalized === "replace" || normalized === "inplace" || normalized === "inline") {
            return "replace";
        }
        return "";
    }

    function usePopupDisplay() {
        return normalizeDisplayMode(config.displayMode) === "popup";
    }

    function syncConfigManager() {
        if (typeof ConfigManager === "undefined") {
            return;
        }
        ConfigManager.wglOllamaTranslateEnabled = config.enabled;
        ConfigManager.wglOllamaTranslateDisplayMode = normalizeDisplayMode(config.displayMode) || "replace";
        ConfigManager.wglOllamaTranslateDialogue = config.translateDialogue;
        ConfigManager.wglOllamaTranslateChoices = config.translateChoices;
        ConfigManager.wglOllamaTranslateWindowText = config.translateWindowText;
    }

    function isOllamaOptionSymbol(symbol) {
        return symbol === "wglOllamaTranslateEnabled" ||
            symbol === "wglOllamaTranslateDisplayMode" ||
            symbol === "wglOllamaTranslateDialogue" ||
            symbol === "wglOllamaTranslateChoices" ||
            symbol === "wglOllamaTranslateWindowText";
    }

    function applyOllamaOptionValue(symbol, value) {
        switch (symbol) {
        case "wglOllamaTranslateEnabled":
            config.enabled = !!value;
            break;
        case "wglOllamaTranslateDisplayMode":
            config.displayMode = normalizeDisplayMode(value) || "replace";
            break;
        case "wglOllamaTranslateDialogue":
            config.translateDialogue = !!value;
            break;
        case "wglOllamaTranslateChoices":
            config.translateChoices = !!value;
            break;
        case "wglOllamaTranslateWindowText":
            config.translateWindowText = !!value;
            break;
        default:
            return;
        }
        syncConfigManager();
        saveStoredConfig();
        refreshPopupButton();
    }

    function shouldSkip(text) {
        const value = String(text || "").trim();
        if (!value) {
            return true;
        }
        if (/^[\s\d.,+\-*/()[\]{}:;!?'"%$#@&_=<>|\\]+$/.test(value)) {
            return true;
        }
        return false;
    }

    function cacheKey(text, context) {
        return [
            config.endpoint,
            config.model,
            config.sourceLanguage,
            config.targetLanguage,
            context,
            String(text || "")
        ].join("\u0000");
    }

    function remember(key, value) {
        if (!Object.prototype.hasOwnProperty.call(cache, key)) {
            cacheKeys.push(key);
            if (cacheKeys.length > maxCacheEntries) {
                delete cache[cacheKeys.shift()];
            }
        }
        cache[key] = value;
    }

    function cleanTranslatedText(text) {
        let value = String(text || "").trim();
        value = value.replace(/^```(?:text)?\s*/i, "").replace(/\s*```$/i, "").trim();
        value = extractJSONTranslation(value).translation;
        value = stripCommonPrefixes(value);
        value = stripWrappedQuotes(value);
        return value.trim();
    }

    function extractJSONTranslation(text) {
        const value = String(text || "").trim();
        if (!value) {
            return {translation: "", confidence: 0};
        }
        try {
            const parsed = JSON.parse(value);
            if (parsed && typeof parsed.translation === "string") {
                return {
                    translation: parsed.translation,
                    confidence: normalizeConfidence(parsed.confidence)
                };
            }
            if (parsed && typeof parsed.text === "string") {
                return {
                    translation: parsed.text,
                    confidence: normalizeConfidence(parsed.confidence)
                };
            }
        } catch (error) {
            // Fall through to regex extraction.
        }

        const match = value.match(/"translation"\s*:\s*"((?:\\.|[^"\\])*)"/);
        if (match) {
            try {
                return {
                    translation: JSON.parse("\"" + match[1] + "\""),
                    confidence: extractConfidence(value)
                };
            } catch (error) {
                return {
                    translation: match[1],
                    confidence: extractConfidence(value)
                };
            }
        }
        return {translation: value, confidence: 1};
    }

    function extractConfidence(text) {
        const match = String(text || "").match(/"confidence"\s*:\s*([0-9.]+)/);
        return match ? normalizeConfidence(Number(match[1])) : 1;
    }

    function normalizeConfidence(value) {
        const confidence = Number(value);
        if (!Number.isFinite(confidence)) {
            return 1;
        }
        if (confidence > 1 && confidence <= 100) {
            return confidence / 100;
        }
        return Math.max(0, Math.min(1, confidence));
    }

    function stripCommonPrefixes(text) {
        let value = String(text || "").trim();
        value = value.replace(/^(?:translation|translated text|answer|output|result)\s*[:：]\s*/i, "");
        value = value.replace(/^Here(?:'s| is) (?:the )?translation\s*[:：]\s*/i, "");
        value = value.replace(/^The translation is\s*[:：]?\s*/i, "");
        const lines = value.split(/\n+/).map(function(line) {
            return line.trim();
        }).filter(function(line) {
            return line.length > 0 && !/^(?:note|explanation|literal|context)\s*[:：]/i.test(line);
        });
        if (lines.length > 0) {
            value = lines[0];
        }
        return value.trim();
    }

    function stripWrappedQuotes(text) {
        let value = String(text || "").trim();
        if ((value.charAt(0) === "\"" && value.charAt(value.length - 1) === "\"") ||
                (value.charAt(0) === "'" && value.charAt(value.length - 1) === "'")) {
            value = value.slice(1, -1).trim();
        }
        return value;
    }

    function maxCharactersForContext(context) {
        switch (context) {
        case "dialogue":
        case "connection test":
            return config.maxDialogueCharacters;
        case "choice":
            return config.maxChoiceCharacters;
        case "window text":
            return config.maxWindowTextCharacters;
        default:
            return config.maxDialogueCharacters;
        }
    }

    function shortenText(text, maxCharacters) {
        let value = String(text || "").replace(/\s+/g, " ").trim();
        if (!maxCharacters || value.length <= maxCharacters) {
            return value;
        }

        const sentenceMatch = value.match(/^(.{1,}?[.!?])\s+/);
        if (sentenceMatch && sentenceMatch[1].length <= maxCharacters) {
            value = sentenceMatch[1].trim();
        }
        if (value.length <= maxCharacters) {
            return value;
        }

        const clipped = value.slice(0, Math.max(1, maxCharacters - 1));
        const lastSpace = clipped.lastIndexOf(" ");
        if (lastSpace > Math.floor(maxCharacters * 0.6)) {
            return clipped.slice(0, lastSpace).trim() + "...";
        }
        return clipped.trim() + "...";
    }

    function fitTranslatedText(text, context) {
        return shortenText(cleanTranslatedText(text), maxCharactersForContext(context));
    }

    function fitTranslationResponse(text, original, context) {
        const protectedOriginal = protectMarkup(original);
        const extracted = extractJSONTranslation(String(text || "").replace(/^```(?:json|text)?\s*/i, "").replace(/\s*```$/i, "").trim());
        if (extracted.confidence < config.minimumConfidence) {
            return String(original || "");
        }

        let cleaned = stripCommonPrefixes(extracted.translation);
        cleaned = stripWrappedQuotes(cleaned);
        cleaned = restoreMarkup(cleaned, protectedOriginal.tokens);
        if (isUncertainText(cleaned)) {
            return String(original || "");
        }
        if (!hasRequiredMarkup(cleaned, protectedOriginal.tokens)) {
            return String(original || "");
        }
        cleaned = shortenText(cleaned, maxCharactersForContext(context));
        cleaned = restoreMarkup(cleaned, protectedOriginal.tokens);
        if (!hasRequiredMarkup(cleaned, protectedOriginal.tokens)) {
            return String(original || "");
        }
        if (!cleaned) {
            return String(original || "");
        }
        return cleaned;
    }

    function isUncertainText(text) {
        return /\b(?:not sure|unsure|uncertain|cannot translate|can't translate|unable to translate|need more context|ambiguous)\b/i.test(String(text || ""));
    }

    function protectMarkup(text) {
        const tokens = [];
        const value = String(text || "").replace(/(\\[A-Za-z]+(?:\[[^\]]*\])?|<[^>\n]+>)/g, function(match) {
            const placeholder = "__WGL_TOKEN_" + tokens.length + "__";
            tokens.push({
                placeholder: placeholder,
                value: match
            });
            return placeholder;
        });
        return {
            text: value,
            tokens: tokens
        };
    }

    function restoreMarkup(text, tokens) {
        let value = String(text || "");
        for (let i = 0; i < tokens.length; i += 1) {
            const token = tokens[i];
            value = value.split(token.placeholder).join(token.value);
        }
        return value;
    }

    function hasRequiredMarkup(text, tokens) {
        const value = String(text || "");
        for (let i = 0; i < tokens.length; i += 1) {
            if (value.indexOf(tokens[i].value) === -1) {
                return false;
            }
        }
        return true;
    }

    function promptFor(text, context) {
        const maxCharacters = maxCharactersForContext(context);
        const protectedText = protectMarkup(text);
        return [
            "You are a translation engine for RPG Maker UI text.",
            "Translate from " + config.sourceLanguage + " to " + config.targetLanguage + ".",
            "Return valid compact JSON only: {\"translation\":\"...\",\"confidence\":0.0}",
            "The translation value must be at most " + maxCharacters + " characters.",
            "Confidence must be between 0 and 1.",
            "If unsure, ambiguous, idiomatic, garbled, or not confident, return the original text exactly and confidence 0.",
            "Keep every placeholder token like __WGL_TOKEN_0__ exactly unchanged.",
            "Those placeholder tokens are control codes, icons, colors, names, or markup and must not be translated.",
            "Preserve line breaks.",
            "Do not add explanations, notes, markdown, labels, alternatives, romanization, or source text.",
            "Prefer short natural phrasing that fits a game text box.",
            "Context: " + context,
            "",
            protectedText.text
        ].join("\n");
    }

    function showStatus(message) {
        const text = String(message || "").trim();
        if (!text) {
            return;
        }
        if (typeof window !== "undefined" && typeof window.alert === "function") {
            window.alert(text);
        } else {
            console.log("WGLOllamaTranslate:", text);
        }
    }

    function ensurePopupElements() {
        if (typeof document === "undefined" || !document.body) {
            return false;
        }
        if (popupButton && popupPanel) {
            return true;
        }

        popupButton = document.createElement("button");
        popupButton.type = "button";
        popupButton.textContent = "i";
        popupButton.title = "Show Ollama translation";
        popupButton.setAttribute("aria-label", "Show Ollama translation");
        popupButton.style.position = "fixed";
        popupButton.style.right = "18px";
        popupButton.style.bottom = "18px";
        popupButton.style.zIndex = "999998";
        popupButton.style.width = "34px";
        popupButton.style.height = "34px";
        popupButton.style.border = "1px solid rgba(255,255,255,0.7)";
        popupButton.style.borderRadius = "17px";
        popupButton.style.background = "rgba(18, 22, 30, 0.86)";
        popupButton.style.color = "#ffffff";
        popupButton.style.font = "bold 20px sans-serif";
        popupButton.style.lineHeight = "30px";
        popupButton.style.cursor = "pointer";
        popupButton.style.display = "none";
        popupButton.addEventListener("click", function(event) {
            event.preventDefault();
            event.stopPropagation();
            toggleTranslationPopup();
        });

        popupPanel = document.createElement("div");
        popupPanel.setAttribute("role", "dialog");
        popupPanel.style.position = "fixed";
        popupPanel.style.right = "18px";
        popupPanel.style.left = "auto";
        popupPanel.style.bottom = "62px";
        popupPanel.style.zIndex = "999997";
        popupPanel.style.width = "calc(100vw - 36px)";
        popupPanel.style.maxWidth = "520px";
        popupPanel.style.maxHeight = "45vh";
        popupPanel.style.overflow = "auto";
        popupPanel.style.display = "none";
        popupPanel.style.boxSizing = "border-box";
        popupPanel.style.padding = "12px 14px";
        popupPanel.style.border = "1px solid rgba(255,255,255,0.24)";
        popupPanel.style.borderRadius = "6px";
        popupPanel.style.background = "rgba(18, 22, 30, 0.94)";
        popupPanel.style.color = "#ffffff";
        popupPanel.style.font = "16px/1.45 sans-serif";
        popupPanel.style.whiteSpace = "pre-wrap";
        popupPanel.style.overflowWrap = "break-word";
        popupPanel.style.boxShadow = "0 8px 24px rgba(0,0,0,0.35)";
        popupPanel.addEventListener("click", function(event) {
            event.stopPropagation();
        });

        document.body.appendChild(popupPanel);
        document.body.appendChild(popupButton);
        return true;
    }

    function clearNode(node) {
        while (node.firstChild) {
            node.removeChild(node.firstChild);
        }
    }

    function appendPopupEntry(container, entry) {
        const wrapper = document.createElement("div");
        wrapper.style.marginBottom = "10px";

        const label = document.createElement("div");
        label.textContent = entry.context;
        label.style.marginBottom = "4px";
        label.style.color = "rgba(255,255,255,0.68)";
        label.style.font = "12px/1.2 sans-serif";
        label.style.textTransform = "uppercase";

        const text = document.createElement("div");
        text.textContent = entry.pending ? "Translating..." : (entry.translation || entry.original || "");

        wrapper.appendChild(label);
        wrapper.appendChild(text);
        container.appendChild(wrapper);
    }

    function renderTranslationPopup() {
        if (!popupPanel) {
            return;
        }
        clearNode(popupPanel);
        for (let i = 0; i < latestPopupEntries.length; i += 1) {
            appendPopupEntry(popupPanel, latestPopupEntries[i]);
        }
    }

    function clampPopupPanelToViewport() {
        if (!popupPanel || typeof popupPanel.getBoundingClientRect !== "function") {
            return;
        }

        const margin = 18;
        const viewportWidth = Math.max(document.documentElement.clientWidth || 0, window.innerWidth || 0);
        if (!viewportWidth) {
            return;
        }

        const rect = popupPanel.getBoundingClientRect();
        if (rect.right > viewportWidth - margin) {
            popupPanel.style.left = "auto";
            popupPanel.style.right = margin + "px";
        } else if (rect.left < margin) {
            popupPanel.style.left = margin + "px";
            popupPanel.style.right = margin + "px";
            popupPanel.style.width = "auto";
        }
    }

    function hasPendingPopupTranslations() {
        return latestPopupEntries.some(function(entry) {
            return !!entry.pending;
        });
    }

    function translatePendingPopupEntries() {
        let changed = false;
        for (let i = 0; i < latestPopupEntries.length; i += 1) {
            const entry = latestPopupEntries[i];
            if (!entry.pending) {
                continue;
            }
            entry.translation = translate(entry.original, entry.context, true);
            entry.pending = false;
            changed = true;
        }
        return changed;
    }

    function showTranslationPopup() {
        if (!usePopupDisplay() || latestPopupEntries.length === 0 || !ensurePopupElements()) {
            return;
        }
        renderTranslationPopup();
        popupPanel.style.display = "block";
        clampPopupPanelToViewport();

        if (hasPendingPopupTranslations()) {
            window.setTimeout(function() {
                if (translatePendingPopupEntries() && popupPanel && popupPanel.style.display === "block") {
                    renderTranslationPopup();
                    clampPopupPanelToViewport();
                }
            }, 0);
        }
    }

    function hideTranslationPopup() {
        if (popupPanel) {
            popupPanel.style.display = "none";
        }
    }

    function toggleTranslationPopup() {
        if (popupPanel && popupPanel.style.display === "block") {
            hideTranslationPopup();
            return;
        }
        showTranslationPopup();
    }

    function refreshPopupButton() {
        if (!ensurePopupElements()) {
            return;
        }
        popupButton.style.display = usePopupDisplay() && latestPopupEntries.length > 0 ? "block" : "none";
        if (!usePopupDisplay()) {
            hideTranslationPopup();
        }
    }

    function rememberPopupText(original, context) {
        if (!usePopupDisplay()) {
            return;
        }

        const entry = {
            context: String(context || "text"),
            original: String(original || ""),
            translation: "",
            pending: true
        };

        if (context === "dialogue") {
            latestPopupEntries = [entry];
        } else if (context === "choice") {
            latestPopupEntries = latestPopupEntries.filter(function(existing) {
                return existing.context === "dialogue" || existing.context === "choice";
            });
            latestPopupEntries.push(entry);
        } else if (latestPopupEntries.length === 0) {
            latestPopupEntries = [entry];
        }

        if (latestPopupEntries.length > 8) {
            latestPopupEntries = latestPopupEntries.slice(latestPopupEntries.length - 8);
        }
        refreshPopupButton();
    }

    function requestWithCurl(payload) {
        if (typeof require !== "function") {
            throw new Error("Node require is unavailable");
        }

        const childProcess = require("child_process");
        const response = childProcess.execFileSync(config.curlPath || "curl", [
            "-sS",
            "--max-time",
            "30",
            "-H",
            "Content-Type: application/json",
            "-X",
            "POST",
            "-d",
            JSON.stringify(payload),
            config.endpoint
        ], {
            encoding: "utf8",
            windowsHide: true
        });
        return JSON.parse(response || "{}");
    }

    function requestWithXHR(payload) {
        const xhr = new XMLHttpRequest();
        xhr.open("POST", config.endpoint, false);
        xhr.setRequestHeader("Content-Type", "application/json");
        xhr.send(JSON.stringify(payload));

        if (xhr.status < 200 || xhr.status >= 300) {
            throw new Error("HTTP " + xhr.status + ": " + String(xhr.responseText || "").slice(0, 300));
        }

        return JSON.parse(xhr.responseText || "{}");
    }

    function requestOllama(payload) {
        const errors = [];

        try {
            return requestWithCurl(payload);
        } catch (error) {
            errors.push("curl: " + (error && error.message ? error.message : String(error)));
        }

        try {
            return requestWithXHR(payload);
        } catch (error) {
            errors.push("xhr: " + (error && error.message ? error.message : String(error)));
        }

        throw new Error(errors.join("\n"));
    }

    function ollamaPayload(text, context) {
        return {
            model: config.model,
            prompt: promptFor(text, context),
            stream: false,
            format: "json",
            options: {
                temperature: 0,
                num_predict: 96
            }
        };
    }

    function updateStatus(ok, message) {
        lastStatus = ok ? "ok" : "error";
        lastError = ok ? "" : String(message || "");
        if (!ok) {
            console.warn("WGLOllamaTranslate request failed:", lastError);
        }
    }

    function testConnection() {
        try {
            const parsed = requestOllama(ollamaPayload("こんにちは", "connection test"));
            const translated = fitTranslationResponse(parsed.response || "", "こんにちは", "connection test");
            updateStatus(true, "");
            return "Ollama translation test OK:\n" + (translated || "(empty response)");
        } catch (error) {
            updateStatus(false, error && error.message ? error.message : String(error));
            return "Ollama translation test failed:\n" + lastError;
        }
    }

    function translate(text, context, enabledForContext) {
        const original = String(text || "");
        if (!config.enabled || !enabledForContext || shouldSkip(original)) {
            return original;
        }

        const key = cacheKey(original, context);
        if (Object.prototype.hasOwnProperty.call(cache, key)) {
            return cache[key];
        }

        try {
            const parsed = requestOllama(ollamaPayload(original, context));
            const translated = fitTranslationResponse(parsed.response || "", original, context);
            const result = translated || original;
            remember(key, result);
            updateStatus(true, "");
            return result;
        } catch (error) {
            updateStatus(false, error && error.message ? error.message : String(error));
            remember(key, original);
            return original;
        }
    }

    function translateForDisplay(text, context, enabledForContext) {
        const original = String(text || "");
        if (usePopupDisplay() && config.enabled && enabledForContext && !shouldSkip(original)) {
            rememberPopupText(original, context);
            return original;
        }
        return translate(original, context, enabledForContext);
    }

    function currentMessageWindow() {
        if (typeof SceneManager === "undefined" || !SceneManager._scene) {
            return null;
        }
        return SceneManager._scene._messageWindow || null;
    }

    function eventCanvasPoint(event) {
        if (typeof Graphics !== "undefined" &&
                typeof Graphics.pageToCanvasX === "function" &&
                typeof Graphics.pageToCanvasY === "function") {
            return {
                x: Graphics.pageToCanvasX(event.pageX),
                y: Graphics.pageToCanvasY(event.pageY)
            };
        }
        const canvas = typeof Graphics !== "undefined" ? Graphics._canvas : null;
        if (canvas && typeof canvas.getBoundingClientRect === "function") {
            const rect = canvas.getBoundingClientRect();
            const scaleX = canvas.width / Math.max(1, rect.width);
            const scaleY = canvas.height / Math.max(1, rect.height);
            return {
                x: (event.clientX - rect.left) * scaleX,
                y: (event.clientY - rect.top) * scaleY
            };
        }
        return {x: event.pageX || 0, y: event.pageY || 0};
    }

    function isPointInsideWindow(gameWindow, x, y) {
        if (!gameWindow || !gameWindow.visible) {
            return false;
        }
        if (typeof gameWindow.isOpen === "function" && !gameWindow.isOpen()) {
            return false;
        }
        const width = Number(gameWindow.width || 0);
        const height = Number(gameWindow.height || 0);
        const left = Number(gameWindow.x || 0);
        const top = Number(gameWindow.y || 0);
        return width > 0 && height > 0 && x >= left && y >= top && x <= left + width && y <= top + height;
    }

    function maybeShowPopupForMessageClick(event) {
        if (!usePopupDisplay() || latestPopupEntries.length === 0 || event.button > 0) {
            return;
        }
        const point = eventCanvasPoint(event);
        if (isPointInsideWindow(currentMessageWindow(), point.x, point.y)) {
            showTranslationPopup();
        }
    }

    if (typeof Game_Message !== "undefined" && Game_Message.prototype) {
        if (typeof Game_Message.prototype.add === "function") {
            const originalGameMessageAdd = Game_Message.prototype.add;
            Game_Message.prototype.add = function(text) {
                if (config.translateDialogue && typeof window !== "undefined" &&
                        typeof window.__wglCaptureOriginalMessage === "function") {
                    window.__wglCaptureOriginalMessage(text);
                }
                return originalGameMessageAdd.call(
                    this,
                    translateForDisplay(text, "dialogue", config.translateDialogue)
                );
            };
        }

        if (typeof Game_Message.prototype.setChoices === "function") {
            const originalGameMessageSetChoices = Game_Message.prototype.setChoices;
            Game_Message.prototype.setChoices = function(choices, defaultType, cancelType) {
                if (Array.isArray(choices) && config.translateChoices) {
                    choices = choices.map(function(choice) {
                        return translateForDisplay(choice, "choice", true);
                    });
                }
                return originalGameMessageSetChoices.call(this, choices, defaultType, cancelType);
            };
        }
    }

    if (typeof Window_Base !== "undefined" && Window_Base.prototype) {
        if (typeof Window_Base.prototype.drawText === "function") {
            const originalWindowBaseDrawText = Window_Base.prototype.drawText;
            Window_Base.prototype.drawText = function(text, x, y, maxWidth, align) {
                const translated = translateForDisplay(text, "window text", config.translateWindowText);
                return originalWindowBaseDrawText.call(this, translated, x, y, maxWidth, align);
            };
        }

        if (typeof Window_Base.prototype.drawTextEx === "function") {
            const originalWindowBaseDrawTextEx = Window_Base.prototype.drawTextEx;
            Window_Base.prototype.drawTextEx = function(text, x, y, width) {
                const translated = translateForDisplay(text, "window text", config.translateWindowText);
                return originalWindowBaseDrawTextEx.call(this, translated, x, y, width);
            };
        }
    }

    if (typeof TouchInput !== "undefined" && TouchInput) {
        if (typeof TouchInput._onMouseDown === "function") {
            const originalTouchInputOnMouseDown = TouchInput._onMouseDown;
            TouchInput._onMouseDown = function(event) {
                const result = originalTouchInputOnMouseDown.call(this, event);
                maybeShowPopupForMessageClick(event);
                return result;
            };
        }
    }

    if (typeof ConfigManager !== "undefined") {
        syncConfigManager();

        if (typeof ConfigManager.makeData === "function") {
            const originalConfigManagerMakeData = ConfigManager.makeData;
            ConfigManager.makeData = function() {
                const data = originalConfigManagerMakeData.call(this);
                data.wglOllamaTranslateEnabled = config.enabled;
                data.wglOllamaTranslateDisplayMode = normalizeDisplayMode(config.displayMode) || "replace";
                data.wglOllamaTranslateDialogue = config.translateDialogue;
                data.wglOllamaTranslateChoices = config.translateChoices;
                data.wglOllamaTranslateWindowText = config.translateWindowText;
                return data;
            };
        }

        if (typeof ConfigManager.applyData === "function") {
            const originalConfigManagerApplyData = ConfigManager.applyData;
            ConfigManager.applyData = function(configData) {
                originalConfigManagerApplyData.call(this, configData);
                if (configData && typeof configData.wglOllamaTranslateEnabled !== "undefined") {
                    config.enabled = !!configData.wglOllamaTranslateEnabled;
                }
                if (configData && typeof configData.wglOllamaTranslateDisplayMode !== "undefined") {
                    config.displayMode = normalizeDisplayMode(configData.wglOllamaTranslateDisplayMode) || config.displayMode;
                }
                if (configData && typeof configData.wglOllamaTranslateDialogue !== "undefined") {
                    config.translateDialogue = !!configData.wglOllamaTranslateDialogue;
                }
                if (configData && typeof configData.wglOllamaTranslateChoices !== "undefined") {
                    config.translateChoices = !!configData.wglOllamaTranslateChoices;
                }
                if (configData && typeof configData.wglOllamaTranslateWindowText !== "undefined") {
                    config.translateWindowText = !!configData.wglOllamaTranslateWindowText;
                }
                syncConfigManager();
                saveStoredConfig();
                refreshPopupButton();
            };
        }
    }

    if (typeof Window_Options !== "undefined" && Window_Options.prototype) {
        if (typeof Window_Options.prototype.addGeneralOptions === "function") {
            const originalWindowOptionsAddGeneralOptions = Window_Options.prototype.addGeneralOptions;
            Window_Options.prototype.addGeneralOptions = function() {
                originalWindowOptionsAddGeneralOptions.call(this);
                this.addCommand(config.menuCommandName, "wglOllamaTranslateEnabled");
                this.addCommand("Ollama Display", "wglOllamaTranslateDisplayMode");
                this.addCommand("Translate Dialogue", "wglOllamaTranslateDialogue");
                this.addCommand("Translate Choices", "wglOllamaTranslateChoices");
                this.addCommand("Translate Window Text", "wglOllamaTranslateWindowText");
            };
        }

        if (typeof Window_Options.prototype.setConfigValue === "function") {
            const originalWindowOptionsSetConfigValue = Window_Options.prototype.setConfigValue;
            Window_Options.prototype.setConfigValue = function(symbol, value) {
                if (isOllamaOptionSymbol(symbol)) {
                    applyOllamaOptionValue(symbol, value);
                    return;
                }
                originalWindowOptionsSetConfigValue.call(this, symbol, value);
            };
        }

        if (typeof Window_Options.prototype.statusText === "function") {
            const originalWindowOptionsStatusText = Window_Options.prototype.statusText;
            Window_Options.prototype.statusText = function(index) {
                const symbol = this.commandSymbol(index);
                if (symbol === "wglOllamaTranslateDisplayMode") {
                    return usePopupDisplay() ? "Popup" : "Replace";
                }
                return originalWindowOptionsStatusText.call(this, index);
            };
        }

        function changeDisplayMode(optionsWindow, mode) {
            if (optionsWindow && typeof optionsWindow.changeValue === "function") {
                optionsWindow.changeValue("wglOllamaTranslateDisplayMode", mode);
            } else {
                applyOllamaOptionValue("wglOllamaTranslateDisplayMode", mode);
            }
        }

        if (typeof Window_Options.prototype.processOk === "function") {
            const originalWindowOptionsProcessOk = Window_Options.prototype.processOk;
            Window_Options.prototype.processOk = function() {
                const symbol = this.commandSymbol(this.index());
                if (symbol === "wglOllamaTranslateDisplayMode") {
                    changeDisplayMode(this, usePopupDisplay() ? "replace" : "popup");
                    return;
                }
                originalWindowOptionsProcessOk.call(this);
            };
        }

        if (typeof Window_Options.prototype.cursorRight === "function") {
            const originalWindowOptionsCursorRight = Window_Options.prototype.cursorRight;
            Window_Options.prototype.cursorRight = function(wrap) {
                const symbol = this.commandSymbol(this.index());
                if (symbol === "wglOllamaTranslateDisplayMode") {
                    changeDisplayMode(this, "popup");
                    return;
                }
                originalWindowOptionsCursorRight.call(this, wrap);
            };
        }

        if (typeof Window_Options.prototype.cursorLeft === "function") {
            const originalWindowOptionsCursorLeft = Window_Options.prototype.cursorLeft;
            Window_Options.prototype.cursorLeft = function(wrap) {
                const symbol = this.commandSymbol(this.index());
                if (symbol === "wglOllamaTranslateDisplayMode") {
                    changeDisplayMode(this, "replace");
                    return;
                }
                originalWindowOptionsCursorLeft.call(this, wrap);
            };
        }
    }

    if (typeof Window_MenuCommand !== "undefined" && Window_MenuCommand.prototype) {
        if (typeof Window_MenuCommand.prototype.addOriginalCommands === "function") {
            const originalWindowMenuCommandAddOriginalCommands = Window_MenuCommand.prototype.addOriginalCommands;
            Window_MenuCommand.prototype.addOriginalCommands = function() {
                originalWindowMenuCommandAddOriginalCommands.call(this);
                if (config.showMenuCommand) {
                    this.addCommand(config.menuCommandName, "wglOllamaTranslate", true);
                }
            };
        }
    }

    if (typeof Scene_Menu !== "undefined" && Scene_Menu.prototype) {
        if (typeof Scene_Menu.prototype.createCommandWindow === "function") {
            const originalSceneMenuCreateCommandWindow = Scene_Menu.prototype.createCommandWindow;
            Scene_Menu.prototype.createCommandWindow = function() {
                originalSceneMenuCreateCommandWindow.call(this);
                if (this._commandWindow && typeof this._commandWindow.setHandler === "function") {
                    this._commandWindow.setHandler("wglOllamaTranslate", this.commandWGLOllamaTranslate.bind(this));
                }
            };
        }

        Scene_Menu.prototype.commandWGLOllamaTranslate = function() {
            openConfigPrompts();
            if (this._commandWindow && typeof this._commandWindow.activate === "function") {
                this._commandWindow.activate();
            }
        };
    }

    window.__wglOllamaTranslate = {
        config: config,
        cache: cache,
        status: function() {
            return {status: lastStatus, error: lastError};
        },
        clearCache: clearCache,
        openConfig: openConfigPrompts,
        testConnection: testConnection,
        showPopup: showTranslationPopup,
        hidePopup: hideTranslationPopup,
        translate: function(text) {
            return translate(text, "manual text", true);
        }
    };
})();
