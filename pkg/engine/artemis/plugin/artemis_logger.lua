local function quote(s)
    s = tostring(s or "")
    s = string.gsub(s, "\\", "\\\\")
    s = string.gsub(s, '"', '\\"')
    return '"' .. s .. '"'
end

local function call_log(msg)
    if type(os) ~= "table" or type(os.execute) ~= "function" then
        return false
    end

    return os.execute("log.exe " .. quote(msg))
end

pcall(call_log, "ヨムナ Artemis logger loaded from artemis_logger.lua")