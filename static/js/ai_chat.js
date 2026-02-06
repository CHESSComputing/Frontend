let inactivityTimer;
const maxMessages = 20;
const inactivityTimeout = 5 * 60 * 1000; // 5 min

function initChatWidget() {
    const chatButton = document.getElementById('chat-button');
    const chatWindow = document.getElementById('chat-window');
    const chatMessages = document.getElementById('chat-messages');
    const chatInput = document.getElementById('chat-user-input');
    const sendButton = document.getElementById('send-chat');
    const minimizeButton = document.getElementById('minimize-chat');
    const closeButton = document.getElementById('close-chat');

    chatButton.addEventListener('click', () => toggleChat(chatWindow));
    minimizeButton.addEventListener('click', () => minimizeChat(chatWindow));
    closeButton.addEventListener('click', () => closeChat(chatWindow, chatMessages));
    sendButton.addEventListener('click', () => sendMessage(chatInput, chatMessages));
    chatInput.addEventListener('keypress', e => {
        if (e.key === 'Enter') sendMessage(chatInput, chatMessages);
    });
}

function toggleChat(chatWindow) {
    if (chatWindow.style.display === 'flex') {
        minimizeChat(chatWindow);
    } else {
        openChat(chatWindow);
    }
}


function openChat(chatWindow) {
    chatWindow.style.display = 'flex';
    resetInactivityTimer();
}

function minimizeChat(chatWindow) {
    chatWindow.style.display = 'none';
}

function closeChat(chatWindow, chatMessages) {
    chatWindow.style.display = 'none';
    chatMessages.innerHTML = '<div class="chat-message bot">Hello, I\'m your AI assistance. Please put your question in the chat prompt.</div>';
    clearTimeout(inactivityTimer);
}

function resetInactivityTimer() {
    clearTimeout(inactivityTimer);
    inactivityTimer = setTimeout(() => {
        const chatMessages = document.getElementById('chat-messages');
        chatMessages.innerHTML = '<div class="chat-message bot">Session expired. Please refresh to start again.</div>';
    }, inactivityTimeout);
}

function appendMessage(text, sender, chatMessages) {
    const msg = document.createElement('div');
    msg.classList.add('chat-message', sender);
    msg.textContent = text;
    chatMessages.appendChild(msg);
    chatMessages.scrollTop = chatMessages.scrollHeight;

    const msgs = chatMessages.querySelectorAll('.chat-message');
    if (msgs.length > maxMessages) {
        chatMessages.removeChild(msgs[0]);
    }
}

function sendMessage(chatInput, chatMessages) {
    const userInput = chatInput.value.trim();
    if (!userInput) return;

    appendMessage(userInput, 'user', chatMessages);
    chatInput.value = '';
    resetInactivityTimer();

    // Add loading indicator
    const loadingMsg = document.createElement('div');
    loadingMsg.classList.add('chat-message', 'bot');
    loadingMsg.innerHTML = '<span class="loading-dots">...</span>';
    chatMessages.appendChild(loadingMsg);
    chatMessages.scrollTop = chatMessages.scrollHeight;

    fetch('/aichat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: userInput })
    })
    .then(async response => {
        const data = await response.json();

        // if backend signals error (HTTP or logical)
        if (!response.ok) {
            throw new Error(data.reply || 'Server error');
        }

        return data;
    })
    .then(data => {
        const reply = data.reply || '';

        const hasHTML = /<\/?[a-z][\s\S]*>/i.test(reply);
        if (hasHTML) {
            loadingMsg.innerHTML = reply;
        } else {
            loadingMsg.textContent = reply;
        }
    })
    .catch(err => {
        loadingMsg.textContent = err.message || 'Error contacting server.';
    });
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', initChatWidget);

