// Lists a tiny in-memory catalog of example files from the server console.
#include <sourcemod>
#pragma semicolon 1
#pragma newdecls required

char g_Names[][] = { "notes.md", "letter.eml" };
int g_Sizes[] = { 120, 240 };

public Plugin myinfo =
{
    name = "Catalog",
    author = "Example",
    description = "Lists example files",
    version = "1.0",
    url = "https://example.com"
};

public void OnPluginStart()
{
    RegConsoleCmd("sm_catalog", Command_Catalog);
}

public Action Command_Catalog(int client, int args)
{
    for (int i = 0; i < sizeof(g_Sizes); i++)
    {
        ReplyToCommand(client, "%s: %d bytes", g_Names[i], g_Sizes[i]);
    }
    return Plugin_Handled;
}
