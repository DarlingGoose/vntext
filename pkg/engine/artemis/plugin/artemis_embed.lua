if __ymn_artemis_backlog_logger_loaded ~= true then
__ymn_artemis_backlog_logger_loaded = true
local function __ymn_quote(s)
    s = tostring(s or "")
    s = string.gsub(s, "\\", "\\\\")
    s = string.gsub(s, '"', '\\"')
    return '"' .. s .. '"'
end
local function __ymn_log(msg, speaker, voice)
    msg = tostring(msg or "")
    if msg == "" then return end

    local path = "vntext.log"
    local ok, f = pcall(io.open, path, "a")
    if not ok or f == nil then return end

    local ts = ""
    if type(os) == "table" and type(os.date) == "function" then
        ts = os.date("[%Y-%m-%dT%H:%M:%S]")
    else
        ts = "[unknown-time]"
    end

    f:write(ts)
    if speaker ~= nil and tostring(speaker) ~= "" then
        f:write("[speaker:")
        f:write(tostring(speaker))
        f:write("]")
    end
    if voice ~= nil and tostring(voice) ~= "" then
        f:write("[voice:")
        f:write(tostring(voice))
        f:write("]")
    end
    f:write(": ")
    f:write(msg)
    f:write("\n")
    f:close()
end
local function __ymn_clean(s)
    s = tostring(s or "")
    s = string.gsub(s, "\r", "")
    s = string.gsub(s, "\n", "\\n")
    s = string.gsub(s, "^%s+", "")
    s = string.gsub(s, "%s+$", "")
    return s
end
local __ymn_current_speaker = ""
local __ymn_last_text = ""
if type(set_backlog_name) == "function" and not __ymn_original_set_backlog_name then
    __ymn_original_set_backlog_name = set_backlog_name
    set_backlog_name = function(name)
        __ymn_current_speaker = __ymn_clean(name)
        return __ymn_original_set_backlog_name(name)
    end
    __ymn_log("[system]Yomuna wrapped set_backlog_name")
end
if type(set_backlog_text) == "function" and not __ymn_original_set_backlog_text then
    __ymn_original_set_backlog_text = set_backlog_text
    set_backlog_text = function(com, param)
        if type(param) == "table" then
            local text = __ymn_clean(param.data or param.text or param["0"] or "")
            if text ~= "" and text ~= __ymn_last_text then
                __ymn_last_text = text
                __ymn_log(text, __ymn_current_speaker, "")
            end
        end
        return __ymn_original_set_backlog_text(com, param)
    end
    __ymn_log("[system]Yomuna wrapped set_backlog_text")
end
__ymn_log("[system]Yomuna Artemis backlog logger loaded")
end