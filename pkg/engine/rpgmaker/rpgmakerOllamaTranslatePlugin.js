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
 * This plugin uses synchronous local HTTP requests so RPG Maker can replace
 * text before it is drawn. Repeated strings are cached for the current game
 * session.
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

    const config = {
        enabled: boolParam("Enabled", true),
        endpoint: param("Endpoint", "http://127.0.0.1:11434/api/generate"),
        model: param("Model", "llama3.1"),
        curlPath: param("Curl Path", "curl"),
        sourceLanguage: param("Source Language", "Japanese"),
        targetLanguage: param("Target Language", "English"),
        translateDialogue: boolParam("Translate Dialogue", true),
        translateChoices: boolParam("Translate Choices", true),
        translateWindowText: boolParam("Translate Window Text", true),
        showMenuCommand: boolParam("Show Menu Command", true),
        menuCommandName: param("Menu Command Name", "Ollama Translation")
    };

    const storageKey = "WGLOllamaTranslate.config";
    const cache = {};
    const cacheKeys = [];
    const maxCacheEntries = 1000;
    let lastStatus = "not tested";
    let lastError = "";

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

    function openConfigPrompts() {
        config.enabled = boolPrompt("Enable Ollama translation?", config.enabled);
        config.endpoint = stringPrompt("Ollama endpoint", config.endpoint);
        config.model = stringPrompt("Ollama model", config.model);
        config.curlPath = stringPrompt("curl path", config.curlPath);
        config.sourceLanguage = stringPrompt("Source language", config.sourceLanguage);
        config.targetLanguage = stringPrompt("Target language", config.targetLanguage);
        config.translateDialogue = boolPrompt("Translate dialogue?", config.translateDialogue);
        config.translateChoices = boolPrompt("Translate choices?", config.translateChoices);
        config.translateWindowText = boolPrompt("Translate window/menu text?", config.translateWindowText);
        saveStoredConfig();
        clearCache();
        if (typeof ConfigManager !== "undefined") {
            ConfigManager.wglOllamaTranslateEnabled = config.enabled;
            if (typeof ConfigManager.save === "function") {
                ConfigManager.save();
            }
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
        if ((value.charAt(0) === "\"" && value.charAt(value.length - 1) === "\"") ||
                (value.charAt(0) === "'" && value.charAt(value.length - 1) === "'")) {
            value = value.slice(1, -1).trim();
        }
        return value;
    }

    function promptFor(text, context) {
        return [
            "Translate this RPG Maker " + context + " from " + config.sourceLanguage + " to " + config.targetLanguage + ".",
            "Return only the translated text.",
            "Preserve RPG Maker escape codes such as \\N[1], \\V[1], \\C[2], \\I[3], and line breaks.",
            "Do not add explanations.",
            "",
            String(text || "")
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
            options: {
                temperature: 0.1
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
            const translated = cleanTranslatedText(parsed.response || "");
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
            const translated = cleanTranslatedText(parsed.response || "");
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

    if (typeof Game_Message !== "undefined" && Game_Message.prototype) {
        if (typeof Game_Message.prototype.add === "function") {
            const originalGameMessageAdd = Game_Message.prototype.add;
            Game_Message.prototype.add = function(text) {
                return originalGameMessageAdd.call(
                    this,
                    translate(text, "dialogue", config.translateDialogue)
                );
            };
        }

        if (typeof Game_Message.prototype.setChoices === "function") {
            const originalGameMessageSetChoices = Game_Message.prototype.setChoices;
            Game_Message.prototype.setChoices = function(choices, defaultType, cancelType) {
                if (Array.isArray(choices) && config.translateChoices) {
                    choices = choices.map(function(choice) {
                        return translate(choice, "choice", true);
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
                const translated = translate(text, "window text", config.translateWindowText);
                return originalWindowBaseDrawText.call(this, translated, x, y, maxWidth, align);
            };
        }

        if (typeof Window_Base.prototype.drawTextEx === "function") {
            const originalWindowBaseDrawTextEx = Window_Base.prototype.drawTextEx;
            Window_Base.prototype.drawTextEx = function(text, x, y, width) {
                const translated = translate(text, "window text", config.translateWindowText);
                return originalWindowBaseDrawTextEx.call(this, translated, x, y, width);
            };
        }
    }

    if (typeof ConfigManager !== "undefined") {
        ConfigManager.wglOllamaTranslateEnabled = config.enabled;

        if (typeof ConfigManager.makeData === "function") {
            const originalConfigManagerMakeData = ConfigManager.makeData;
            ConfigManager.makeData = function() {
                const data = originalConfigManagerMakeData.call(this);
                data.wglOllamaTranslateEnabled = config.enabled;
                return data;
            };
        }

        if (typeof ConfigManager.applyData === "function") {
            const originalConfigManagerApplyData = ConfigManager.applyData;
            ConfigManager.applyData = function(configData) {
                originalConfigManagerApplyData.call(this, configData);
                if (configData && typeof configData.wglOllamaTranslateEnabled !== "undefined") {
                    config.enabled = !!configData.wglOllamaTranslateEnabled;
                    this.wglOllamaTranslateEnabled = config.enabled;
                    saveStoredConfig();
                }
            };
        }
    }

    if (typeof Window_Options !== "undefined" && Window_Options.prototype) {
        if (typeof Window_Options.prototype.addGeneralOptions === "function") {
            const originalWindowOptionsAddGeneralOptions = Window_Options.prototype.addGeneralOptions;
            Window_Options.prototype.addGeneralOptions = function() {
                originalWindowOptionsAddGeneralOptions.call(this);
                this.addCommand(config.menuCommandName, "wglOllamaTranslateEnabled");
            };
        }

        if (typeof Window_Options.prototype.setConfigValue === "function") {
            const originalWindowOptionsSetConfigValue = Window_Options.prototype.setConfigValue;
            Window_Options.prototype.setConfigValue = function(symbol, volume) {
                originalWindowOptionsSetConfigValue.call(this, symbol, volume);
                if (symbol === "wglOllamaTranslateEnabled") {
                    config.enabled = !!volume;
                    if (typeof ConfigManager !== "undefined") {
                        ConfigManager.wglOllamaTranslateEnabled = config.enabled;
                    }
                    saveStoredConfig();
                }
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
        translate: function(text) {
            return translate(text, "manual text", true);
        }
    };
})();
