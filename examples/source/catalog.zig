const std = @import("std");

pub fn main() void {
    const files = [_][]const u8{ "notes.md", "letter.eml" };
    for (files) |name| {
        std.debug.print("Preview: {s}\n", .{name});
    }
}
