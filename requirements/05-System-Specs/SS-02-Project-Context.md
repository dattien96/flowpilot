# 1. What is context?

Purpose:
- collect and normalize task-specific context.

## Why we need context?

Because there are many flows that need MCP context.

If not include mcp context when init flow, when run that flow, you will see error.


# 2. What can be a context?
For MVP now, we will support 2 types of context:

## 2.1 Text-based context - Put directly in the flow's definition

This type of context is used to store text-based information.
Users can input by typing text or upload file to provide context.
Or they can paste links to resources to provide context.

## 2.2 MCP context - Belong to the parent project

See [SS-01-Project.md](./SS-01-Project.md) for more information.

Basically, MCP need to be configured once per project to share to all flows in that project.

# 3. Types of MCP for MVP
 We want to support the following types of MCP context for MVP:
- Jira ticket: to read ticket description, comment, status, etc
- Figma: to read design spec, component, etc
- Driver google: to read or write file to google driver
- Firebase: to read or write data to firebase, see crash, analytics, etc
- Telegram: to send notification, receive message, etc