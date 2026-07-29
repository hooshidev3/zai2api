#!/usr/bin/env python3
"""
Thread per-account HTTP client through MiMo call chain.

Changes:
- HandleMimoChat(payload, auth) → HandleMimoChat(payload, auth, client)
- UploadToXiaomi(auth, fileName, fileData, mediaType) → +client
- GetConversationHistory(auth, conversationID) → +client
- CreateConversation(auth, conversationID) → +client
- Replace GlobalHTTPClient.Do with client.Do (using ResolveClient)
- In routes/chat.go: processStream/processNonStream/processAutoUploads/handleModels/handleDirectProxy → +client
- handleChatCompletions reads auth+client from context
"""
import sys
import re

def patch_services(path):
    with open(path, 'r') as f:
        content = f.read()
    
    # 1. HandleMimoChat: add client param + use ResolveClient + use client.Do
    content = re.sub(
        r'func HandleMimoChat\(payload models\.MimoPayload, auth models\.Auth\) \(string, error\) \{',
        'func HandleMimoChat(payload models.MimoPayload, auth models.Auth, client *http.Client) (string, error) {\n\tclient = ResolveClient(client)',
        content
    )
    
    # Replace GlobalHTTPClient.Do with client.Do INSIDE HandleMimoChat
    # We need to be careful — only replace the first occurrence after HandleMimoChat
    # Simple approach: replace all GlobalHTTPClient.Do with client.Do (since we'll
    # add client param to all functions that use it)
    
    # 2. UploadToXiaomi: add client param
    content = re.sub(
        r'func UploadToXiaomi\(auth models\.Auth, fileName string, fileData \[\]byte, mediaType string\) \(\*models\.MultiMedia, error\) \{',
        'func UploadToXiaomi(auth models.Auth, fileName string, fileData []byte, mediaType string, client *http.Client) (*models.MultiMedia, error) {\n\tclient = ResolveClient(client)',
        content
    )
    
    # 3. GetConversationHistory: add client param
    content = re.sub(
        r'func GetConversationHistory\(auth models\.Auth, conversationID string\) \(\[\]DialogItem, error\) \{',
        'func GetConversationHistory(auth models.Auth, conversationID string, client *http.Client) ([]DialogItem, error) {\n\tclient = ResolveClient(client)',
        content
    )
    
    # 4. CreateConversation: add client param
    content = re.sub(
        r'func CreateConversation\(auth models\.Auth, conversationID string\) error \{',
        'func CreateConversation(auth models.Auth, conversationID string, client *http.Client) error {\n\tclient = ResolveClient(client)',
        content
    )
    
    # 5. Replace GlobalHTTPClient.Do with client.Do
    content = content.replace('GlobalHTTPClient.Do(req)', 'client.Do(req)')
    content = content.replace('GlobalHTTPClient.Do(uploadReq)', 'client.Do(uploadReq)')
    content = content.replace('GlobalHTTPClient.Do(parseReq)', 'client.Do(parseReq)')
    
    # 6. Update callers within services: GenerateSummary calls GetConversationHistory
    content = re.sub(
        r'history, err := GetConversationHistory\(auth, conversationID\)',
        'history, err := GetConversationHistory(auth, conversationID, nil)',
        content
    )
    
    with open(path, 'w') as f:
        f.write(content)
    
    print(f"  Patched services: {path}")

def patch_routes(path):
    with open(path, 'r') as f:
        content = f.read()
    
    # 1. handleChatCompletions: read auth+client from context at the start
    # Find "func handleChatCompletions(c *gin.Context) {"
    if 'services.GetAuthFromContext' not in content:
        content = re.sub(
            r'(func handleChatCompletions\(c \*gin\.Context\) \{)',
            r'\1\n\tauth, httpClient := services.GetAuthFromContext(c.Request.Context())\n\tif httpClient == nil {\n\t\thttpClient = services.GlobalHTTPClient\n\t}\n\thttpClient = services.ResolveClient(httpClient)\n\t_ = auth\n',
            content
        )
    
    # 2. processStream: add client param
    content = re.sub(
        r'func processStream\(c \*gin\.Context, body io\.Reader, completionID, model string, userID string, query string\) \{',
        'func processStream(c *gin.Context, body io.Reader, completionID, model string, userID string, query string, client *http.Client) {\n\tclient = services.ResolveClient(client)',
        content
    )
    
    # 3. processNonStream: add client param
    content = re.sub(
        r'func processNonStream\(c \*gin\.Context, body io\.Reader, completionID, model string, cacheKey string, userID string, query string\) \{',
        'func processNonStream(c *gin.Context, body io.Reader, completionID, model string, cacheKey string, userID string, query string, client *http.Client) {\n\tclient = services.ResolveClient(client)',
        content
    )
    
    # 4. processAutoUploads: add client param
    content = re.sub(
        r'func processAutoUploads\(messages \[\]models\.Message, auth models\.Auth\) \[\]models\.MultiMedia \{',
        'func processAutoUploads(messages []models.Message, auth models.Auth, client *http.Client) []models.MultiMedia {\n\tclient = services.ResolveClient(client)',
        content
    )
    
    # 5. Replace services.GlobalHTTPClient.Do with httpClient.Do (we use httpClient
    #    which is the per-account client from handleChatCompletions)
    #    But processStream/processNonStream use their own `client` param now
    content = re.sub(
        r'services\.GlobalHTTPClient\.Do\(req\)',
        'client.Do(req)',
        content
    )
    
    # 6. Update caller of services.HandleMimoChat to pass client
    content = re.sub(
        r'services\.HandleMimoChat\(([^,]+), ([^)]+)\)',
        r'services.HandleMimoChat(\1, \2, httpClient)',
        content
    )
    
    # 7. Update caller of services.UploadToXiaomi
    content = re.sub(
        r'services\.UploadToXiaomi\(([^,]+), ([^,]+), ([^,]+), ([^)]+)\)',
        r'services.UploadToXiaomi(\1, \2, \3, \4, client)',
        content
    )
    
    # 8. Update caller of services.GetConversationHistory
    content = re.sub(
        r'services\.GetConversationHistory\(([^,]+), ([^)]+)\)',
        r'services.GetConversationHistory(\1, \2, httpClient)',
        content
    )
    
    # 9. Update callers of processStream/processNonStream/processAutoUploads
    content = re.sub(
        r'processStream\(c, (body|streamBody), ([^,]+), ([^,]+), ([^,]+), ([^)]+)\)',
        r'processStream(c, \1, \2, \3, \4, \5, httpClient)',
        content
    )
    content = re.sub(
        r'processNonStream\(c, (body|streamBody), ([^,]+), ([^,]+), ([^,]+), ([^,]+), ([^)]+)\)',
        r'processNonStream(c, \1, \2, \3, \4, \5, \6, httpClient)',
        content
    )
    content = re.sub(
        r'processAutoUploads\(messages, auth\)',
        r'processAutoUploads(messages, auth, httpClient)',
        content
    )
    
    # 10. handleModels: read client from context
    if 'func handleModels' in content and 'GetAuthFromContext' not in content.split('func handleModels')[1].split('func')[0]:
        content = re.sub(
            r'(func handleModels\(c \*gin\.Context\) \{)',
            r'\1\n\t_, httpClient := services.GetAuthFromContext(c.Request.Context())\n\thttpClient = services.ResolveClient(httpClient)\n',
            content
        )
        # Replace its GlobalHTTPClient.Do
        # (already handled by regex above for services.GlobalHTTPClient.Do)
    
    # 11. handleDirectProxy: read client from context
    if 'func handleDirectProxy' in content:
        content = re.sub(
            r'(func handleDirectProxy\(c \*gin\.Context\) \{)',
            r'\1\n\t_, httpClient := services.GetAuthFromContext(c.Request.Context())\n\thttpClient = services.ResolveClient(httpClient)\n',
            content
        )
    
    with open(path, 'w') as f:
        f.write(content)
    
    print(f"  Patched routes: {path}")

def main():
    if len(sys.argv) != 3:
        print("Usage: thread-client.py <services/mimo.go> <routes/chat.go>", file=sys.stderr)
        sys.exit(1)
    
    patch_services(sys.argv[1])
    patch_routes(sys.argv[2])

if __name__ == '__main__':
    main()
