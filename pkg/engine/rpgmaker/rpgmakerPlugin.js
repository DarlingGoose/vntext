(function() {
    "use strict";

    const COPY_KEY = "c";
    const LOG_FILENAME = "wgl-dialogue.log";

    window.__wglLastMessage = "";
    window.__wglLastChoices = [];
    window.__wglLastChoiceText = "";
    window.__wglTranscriptPath = "";
    let lastLoggedMessage = "";
    let lastLoggedChoicesKey = "";

    function setLatestMessage(text) {
        if (typeof text !== "string") {
            return;
        }
        window.__wglLastMessage = text;
    }

    function latestMessage() {
        return String(window.__wglLastMessage || "").trim();
    }

    function setLatestChoices(choices) {
        if (!Array.isArray(choices)) {
            window.__wglLastChoices = [];
            window.__wglLastChoiceText = "";
            return;
        }
        const normalized = choices.map(function(choice) {
            return String(choice || "").trim();
        }).filter(function(choice) {
            return choice.length > 0;
        });
        window.__wglLastChoices = normalized;
        window.__wglLastChoiceText = normalized.join("\n");
    }

    function resolveTranscriptPath() {
        try {
            if (typeof require !== "function") {
                return "";
            }

            const path = require("path");
            const roots = [];

            if (typeof __dirname === "string" && __dirname) {
                roots.push(path.resolve(__dirname, "..", ".."));
            }
            if (typeof process !== "undefined" && process && typeof process.cwd === "function") {
                roots.push(process.cwd());
            }
            if (typeof process !== "undefined" && process && typeof process.execPath === "string" && process.execPath) {
                roots.push(path.dirname(process.execPath));
            }
            if (typeof nw !== "undefined" && nw.App && typeof nw.App.startPath === "string" && nw.App.startPath) {
                roots.push(nw.App.startPath);
            }

            const seen = {};
            for (let i = 0; i < roots.length; i += 1) {
                const root = String(roots[i] || "").trim();
                if (!root) {
                    continue;
                }
                const normalized = path.resolve(root);
                if (seen[normalized]) {
                    continue;
                }
                seen[normalized] = true;
                return path.join(normalized, LOG_FILENAME);
            }
        } catch (error) {
            console.warn("WGLClipboardText transcript path lookup failed", error);
        }
        return "";
    }

    function currentSpeakerName() {
        try {
            if ($gameMessage && typeof $gameMessage.speakerName === "function") {
                return String($gameMessage.speakerName() || "").trim();
            }
            if ($gameMessage && typeof $gameMessage._speakerName !== "undefined") {
                return String($gameMessage._speakerName || "").trim();
            }
        } catch (error) {
            console.warn("WGLClipboardText speaker lookup failed", error);
        }
        return "";
    }

    function rawMessageText() {
        try {
            if ($gameMessage && typeof $gameMessage.allText === "function") {
                return String($gameMessage.allText() || "");
            }
            if ($gameMessage && Array.isArray($gameMessage._texts)) {
                return $gameMessage._texts.join("\n");
            }
        } catch (error) {
            console.warn("WGLClipboardText raw message lookup failed", error);
        }
        return "";
    }

    function rawChoices() {
        try {
            if ($gameMessage && typeof $gameMessage.choices === "function") {
                return $gameMessage.choices();
            }
            if ($gameMessage && Array.isArray($gameMessage._choices)) {
                return $gameMessage._choices;
            }
        } catch (error) {
            console.warn("WGLClipboardText raw choice lookup failed", error);
        }
        return [];
    }

    function normalizeMessageText(text, messageWindow) {
        let normalized = String(text || "");
        if (normalized && messageWindow && typeof messageWindow.convertEscapeCharacters === "function") {
            normalized = messageWindow.convertEscapeCharacters(normalized);
        }
        return normalized.replace(/\r\n/g, "\n").trim();
    }

    function normalizeChoiceText(text, messageWindow) {
        return normalizeMessageText(text, messageWindow);
    }

    function refreshLatestMessage(messageWindow) {
        const text = normalizeMessageText(rawMessageText(), messageWindow);
        setLatestMessage(text);
        appendToTranscript(text);
    }

    function refreshLatestChoices(messageWindow) {
        const choices = rawChoices().map(function(choice) {
            return normalizeChoiceText(choice, messageWindow);
        }).filter(function(choice) {
            return choice.length > 0;
        });
        setLatestChoices(choices);
        appendChoicesToTranscript(choices);
    }

    function currentMessageWindow() {
        if (typeof SceneManager === "undefined" || !SceneManager || !SceneManager._scene) {
            return null;
        }
        return SceneManager._scene._messageWindow || null;
    }

    function appendToTranscript(text) {
        const message = String(text || "").trim();
        if (!message || message === lastLoggedMessage) {
            return;
        }

        try {
            if (typeof require !== "function") {
                return;
            }

            const fs = require("fs");
            const logPath = resolveTranscriptPath();
            if (!logPath) {
                return;
            }

            const speaker = currentSpeakerName();
            const timestamp = new Date().toISOString();
            const header = speaker ? "[" + timestamp + "][speaker:" + speaker + "]" : "[" + timestamp + "]";
            const entry = header + message;
            fs.appendFileSync(logPath, entry, "utf8");
            window.__wglTranscriptPath = logPath;
            lastLoggedMessage = message;
        } catch (error) {
            console.warn("WGLClipboardText transcript write failed", error);
        }
    }

    function appendChoicesToTranscript(choices) {
        if (!Array.isArray(choices) || choices.length === 0) {
            lastLoggedChoicesKey = "";
            return;
        }

        const normalized = choices.map(function(choice) {
            return String(choice || "").trim();
        }).filter(function(choice) {
            return choice.length > 0;
        });
        if (normalized.length === 0) {
            lastLoggedChoicesKey = "";
            return;
        }

        const choiceKey = normalized.join("\n");
        if (choiceKey === lastLoggedChoicesKey) {
            return;
        }

        try {
            if (typeof require !== "function") {
                return;
            }

            const fs = require("fs");
            const logPath = resolveTranscriptPath();
            if (!logPath) {
                return;
            }

            const timestamp = new Date().toISOString();
            const lines = normalized.map(function(choice, index) {
                return (index + 1) + ". " + choice;
            });
            const entry = "[" + timestamp + "] [choices]\n" + lines.join("\n");
            fs.appendFileSync(logPath, entry, "utf8");
            window.__wglTranscriptPath = logPath;
            lastLoggedChoicesKey = choiceKey;
        } catch (error) {
            console.warn("WGLClipboardText choice transcript write failed", error);
        }
    }

    function copyToClipboard(text) {
        if (!text) {
            return Promise.resolve(false);
        }

        try {
            if (typeof nw !== "undefined" && nw.Clipboard && typeof nw.Clipboard.get === "function") {
                nw.Clipboard.get().set(text, "text");
                return Promise.resolve(true);
            }
        } catch (error) {
            console.warn("WGLClipboardText nw.js clipboard copy failed", error);
        }

        if (typeof navigator !== "undefined" && navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
            return navigator.clipboard.writeText(text).then(function() {
                return true;
            }).catch(function(error) {
                console.warn("WGLClipboardText navigator clipboard copy failed", error);
                return false;
            });
        }

        return Promise.resolve(false);
    }

    const originalGameMessageClear = Game_Message.prototype.clear;
    Game_Message.prototype.clear = function() {
        originalGameMessageClear.call(this);
        setLatestMessage("");
        setLatestChoices([]);
    };

    const originalGameMessageAdd = Game_Message.prototype.add;
    Game_Message.prototype.add = function(text) {
        originalGameMessageAdd.call(this, text);
        refreshLatestMessage(currentMessageWindow());
    };

    if (typeof Game_Message.prototype.setSpeakerName === "function") {
        const originalGameMessageSetSpeakerName = Game_Message.prototype.setSpeakerName;
        Game_Message.prototype.setSpeakerName = function(speakerName) {
            originalGameMessageSetSpeakerName.call(this, speakerName);
            refreshLatestMessage(currentMessageWindow());
        };
    }

    if (typeof Game_Message.prototype.setChoices === "function") {
        const originalGameMessageSetChoices = Game_Message.prototype.setChoices;
        Game_Message.prototype.setChoices = function(choices, defaultType, cancelType) {
            originalGameMessageSetChoices.call(this, choices, defaultType, cancelType);
            refreshLatestChoices(currentMessageWindow());
        };
    }

    const originalWindowMessageStartMessage = Window_Message.prototype.startMessage;
    Window_Message.prototype.startMessage = function() {
        originalWindowMessageStartMessage.call(this);
        refreshLatestMessage(this);
        refreshLatestChoices(this);
    };

    if (typeof Window_ChoiceList !== "undefined" && Window_ChoiceList.prototype) {
        if (typeof Window_ChoiceList.prototype.start === "function") {
            const originalWindowChoiceListStart = Window_ChoiceList.prototype.start;
            Window_ChoiceList.prototype.start = function() {
                originalWindowChoiceListStart.call(this);
                refreshLatestChoices(currentMessageWindow() || this);
            };
        }

        if (typeof Window_ChoiceList.prototype.refresh === "function") {
            const originalWindowChoiceListRefresh = Window_ChoiceList.prototype.refresh;
            Window_ChoiceList.prototype.refresh = function() {
                originalWindowChoiceListRefresh.call(this);
                refreshLatestChoices(currentMessageWindow() || this);
            };
        }
    }

    document.addEventListener("keydown", function(event) {
        if (!event.ctrlKey || String(event.key || "").toLowerCase() !== COPY_KEY) {
            return;
        }

        const text = latestMessage();
        if (!text) {
            return;
        }

        copyToClipboard(text).then(function(copied) {
            if (copied) {
                console.log("WGLClipboardText copied:", text);
            }
        });
    });
})();