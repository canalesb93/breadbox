export function activateOPButton() {
    document.dispatchEvent(new CustomEvent("OPButtonAdded"));
}
export function encodeOPSaveRequest(saveRequest) {
    const validAutocompleteTypes = [
        "none",
        "name",
        "honorific-prefix",
        "given-name",
        "additional-name",
        "family-name",
        "honorific-suffix",
        "nickname",
        "organization-title",
        "username",
        "new-password",
        "current-password",
        "one-time-code",
        "organization",
        "street-address",
        "address-line1",
        "address-line2",
        "address-line3",
        "address-level1",
        "address-level2",
        "address-level3",
        "address-level4",
        "country",
        "country-name",
        "postal-code",
        "cc-name",
        "cc-given-name",
        "cc-additional-name",
        "cc-family-name",
        "cc-number",
        "cc-exp",
        "cc-exp-month",
        "cc-exp-year",
        "cc-csc",
        "cc-type",
        "transaction-currency",
        "transaction-amount",
        "language",
        "bday",
        "bday-day",
        "bday-month",
        "bday-year",
        "sex",
        "url",
        "tel",
        "tel-country-code",
        "tel-national",
        "tel-area-code",
        "tel-local",
        "tel-local-prefix",
        "tel-local-suffix",
        "tel-extension",
        "email",
        "impp"
    ];
    if (!saveRequest.fields) {
        console.error("Missing 'fields' array in save request.");
        return;
    }
    saveRequest.fields.forEach(field => {
        if (field.autocomplete) {
            if (!validAutocompleteTypes.find(acceptableValue => acceptableValue === field.autocomplete)) {
                console.error(`String '${field.autocomplete}' is an invalid input. Autocomplete type must be one of the following: ${validAutocompleteTypes.join(", ")}.`);
            }
        }
        else {
            console.error("Fields array is missing 'autocomplete' designation.");
        }
    });
    const textEncoder = new TextEncoder();
    const uint8Array = textEncoder.encode(JSON.stringify(saveRequest));
    const numberOfCharacters = uint8Array.length;
    const characterList = [];
    for (let i = 0; i < numberOfCharacters; i++) {
        characterList[i] = String.fromCharCode(uint8Array[i]);
    }
    const stringToEncode = characterList.join("");
    return btoa(stringToEncode);
}
const generateUuid = () => {
    return getGlobal().crypto.getRandomValues(new Uint32Array(1))[0].toString(36);
};
function getGlobal() {
    if (typeof window !== "undefined") {
        return window;
    }
    if (typeof globalThis !== "undefined") {
        return globalThis;
    }
    throw new Error("unable to locate global object");
}
class Button extends HTMLElement {
    constructor() {
        super();
        this.internalId = generateUuid();
        this.errorMessages = [];
        this.ctaUrls = {
            en: "https://1password.com/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            es: "https://1password.com/es/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            de: "https://1password.com/de/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            fr: "https://1password.com/fr/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            it: "https://1password.com/it/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            ja: "https://1password.com/jp/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            ko: "https://1password.com/ko/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            pt: "https://1password.com/pt/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            ru: "https://1password.com/ru/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            "zh-CN": "https://1password.com/zh-cn/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
            "zh-TW": "https://1password.com/zh-tw/landing/save-button/?utm_medium=promo&utm_source=save-button&utm_campaign=save-in-1password",
        };
        document.addEventListener("OPButtonEvent", (e) => {
            const message = e.detail;
            if (message.buttonId === this.internalId) {
                message.error
                    ? this._setModalContent(this._localize(message.header), message.body, true, true)
                    : this._setModalContent(this._localize(message.header), message.body, true);
            }
        });
    }
    _loadFonts() {
        if (document.querySelector('style[data-description="onepassword-font-faces"]')) {
            return;
        }
        const inter = "./src/fonts/Inter-roman.var.woff2";
        const onepasswordFonts = `@font-face {
      font-display: swap;
      font-family: "Inter";
      font-style: normal;
      font-weight: 400 600;
      src: url(${inter}) format("woff2");
    }`;
        const styleTag = document.createElement("style");
        styleTag.dataset.description = "onepassword-font-faces";
        styleTag.appendChild(document.createTextNode(onepasswordFonts));
        document.head.appendChild(styleTag);
    }
    _infoIconSvg(mode) {
        switch (mode) {
            case "dark":
                return `<path fill-rule="evenodd" clip-rule="evenodd" d="M16 8C16 12.4183 12.4183 16 8 16C3.58172 16 0 12.4183 0 8C0 3.58172 3.58172 0 8 0C12.4183 0 16 3.58172 16 8ZM7 8C7 7.44772 7.44772 7 8 7C8.55229 7 9 7.44772 9 8V11C9 11.5523 8.55229 12 8 12C7.44772 12 7 11.5523 7 11V8ZM8 6C8.55229 6 9 5.55228 9 5C9 4.44772 8.55229 4 8 4C7.44772 4 7 4.44772 7 5C7 5.55228 7.44772 6 8 6Z" fill="#85BEFF"/>`;
            default:
                return `<path fill-rule="evenodd" clip-rule="evenodd" d="M8 14C11.3137 14 14 11.3137 14 8C14 4.68629 11.3137 2 8 2C4.68629 2 2 4.68629 2 8C2 11.3137 4.68629 14 8 14ZM16 8C16 12.4183 12.4183 16 8 16C3.58172 16 0 12.4183 0 8C0 3.58172 3.58172 0 8 0C12.4183 0 16 3.58172 16 8ZM7 8C7 7.44772 7.44772 7 8 7C8.55229 7 9 7.44772 9 8V11C9 11.5523 8.55229 12 8 12C7.44772 12 7 11.5523 7 11V8ZM9 5C9 5.55228 8.55229 6 8 6C7.44772 6 7 5.55228 7 5C7 4.44772 7.44772 4 8 4C8.55229 4 9 4.44772 9 5Z" fill="#0364D3"/>`;
        }
    }
    _findSupportedLocaleCode(localeCode) {
        if (localeCode) {
            const supportedLanguages = [
                "de",
                "en",
                "es",
                "fr",
                "it",
                "ja",
                "ko",
                "pt",
                "ru",
                "zh-CN",
                "zh-TW",
            ];
            const foundCode = supportedLanguages.find((code) => localeCode.startsWith(code));
            if (foundCode) {
                return foundCode;
            }
        }
        return "en";
    }
    _validateAttributes(validInputs, attributeName, fallbackValue) {
        if (this.hasAttribute(attributeName)) {
            const userInput = this.getAttribute(attributeName);
            const value = userInput;
            if (!validInputs.find(acceptableValue => acceptableValue === value)) {
                const errorMessage = `"${value}" is an invalid input. ${attributeName} can only be one of the following: ${validInputs.join(", ")}.`;
                this.errorMessages.push(errorMessage);
                console.error(errorMessage);
                return fallbackValue;
            }
            return value;
        }
        return fallbackValue;
    }
    get lang() {
        const languageOverride = this.getAttribute("lang");
        if (languageOverride && this._findSupportedLocaleCode(languageOverride)) {
            return this._findSupportedLocaleCode(languageOverride);
        }
        else if (navigator.language &&
            this._findSupportedLocaleCode(navigator.language)) {
            return this._findSupportedLocaleCode(navigator.language);
        }
        else {
            return "en";
        }
    }
    set lang(localeCode) {
        this.setAttribute("lang", localeCode);
    }
    get class() {
        const validClasses = ["black", "white"];
        return this._validateAttributes(validClasses, "class", null);
    }
    set class(input) {
        this.setAttribute("class", input !== null && input !== void 0 ? input : "");
    }
    get value() {
        return this.getAttribute("value");
    }
    set value(input) {
        this.setAttribute("value", input !== null && input !== void 0 ? input : "");
    }
    get data_onepassword_type() {
        const validTypes = ["login", "credit-card", "api-key"];
        return this._validateAttributes(validTypes, "data-onepassword-type", null);
    }
    set data_onepassword_type(input) {
        this.setAttribute("data_onepassword_type", input !== null && input !== void 0 ? input : "");
    }
    get modal_position() {
        return "top";
    }
    set modal_position(position) {
        this.setAttribute("modal_position", position);
    }
    get visible() {
        return this.hasAttribute("visible");
    }
    set visible(val) {
        val
            ? this.setAttribute("visible", "")
            : this.removeAttribute("visible");
    }
    get color_mode() {
        const validModes = ["light", "dark"];
        return this._validateAttributes(validModes, "data-theme", "light");
    }
    set color_mode(mode) {
        this.setAttribute("color_mode", mode);
    }
    get hide_call_to_action() {
        return this.hasAttribute("hide_call_to_action");
    }
    set hide_call_to_action(val) {
        val
            ? this.setAttribute("hide_call_to_action", "")
            : this.removeAttribute("hide_call_to_action");
    }
    get padding() {
        const validModes = ["compact", "normal", "none"];
        return this._validateAttributes(validModes, "padding", "normal");
    }
    set padding(val) {
        this.setAttribute("padding", val);
    }
    static get observedAttributes() {
        return ["visible", "modal_position", "color_mode", "hide_call_to_action", "padding"];
    }
    attributeChangedCallback(name, oldValue, newValue) {
        var _a, _b, _c, _d, _e, _f;
        if (this.shadowRoot && oldValue !== newValue) {
            switch (name) {
                case "visible":
                case "hide_call_to_action":
                    typeof newValue === "string" ? (_a = this.shadowRoot.querySelector(".wrapper")) === null || _a === void 0 ? void 0 : _a.setAttribute(name, newValue) : (_b = this.shadowRoot.querySelector(".wrapper")) === null || _b === void 0 ? void 0 : _b.removeAttribute("visible");
                    break;
                case "modal_position":
                    (_c = this.shadowRoot.querySelector(".wrapper")) === null || _c === void 0 ? void 0 : _c.setAttribute(name, newValue);
                    break;
                case "color_mode":
                    (_d = this.shadowRoot.querySelector(".wrapper")) === null || _d === void 0 ? void 0 : _d.setAttribute(name, newValue);
                case "padding":
                    (_e = this.shadowRoot.querySelector(".wrapper")) === null || _e === void 0 ? void 0 : _e.setAttribute(name, newValue);
                    break;
                case "modal_position":
                    (_f = this.shadowRoot.querySelector(".wrapper")) === null || _f === void 0 ? void 0 : _f.setAttribute(name, newValue);
                    break;
                default:
                    break;
            }
        }
    }
    _localize(string) {
        return string;
    }
    _elementGenerator(element, content, ariaHidden, className) {
        const newElement = document.createElement(element);
        newElement.innerHTML = content;
        newElement.setAttribute("aria-hidden", ariaHidden.toString());
        className && newElement.setAttribute("class", className);
        return newElement;
    }
    _setModalContent(header, body, hideCta, openModal) {
        var _a, _b, _c, _d;
        const headerNode = (_a = this.shadowRoot) === null || _a === void 0 ? void 0 : _a.querySelector("#op-modal-header");
        const bodyNode = (_b = this.shadowRoot) === null || _b === void 0 ? void 0 : _b.querySelector("#op-modal-body");
        headerNode.innerText = header;
        bodyNode.innerText = "";
        const bodyText = [];
        body.forEach(element => {
            if (!element.url) {
                const textToAdd = document.createTextNode(element.text);
                bodyNode.appendChild(textToAdd);
            }
            else if (element.url) {
                const linkedText = this._elementGenerator("a", `${element.text} `, true);
                linkedText.setAttribute("href", element.url);
                linkedText.setAttribute("tabindex", "3");
                linkedText.setAttribute("target", "_blank");
                linkedText.setAttribute("rel", "noopener noreferrer");
                bodyNode.appendChild(linkedText);
            }
            bodyText.push(element.text);
        });
        const helperText = bodyText.join(" ");
        if (hideCta) {
            this.setAttribute("hide_call_to_action", "");
        }
        headerNode.appendChild(this._elementGenerator("span", helperText, false, "sr-only"));
        if (openModal) {
            const mainButton = (_c = this.shadowRoot) === null || _c === void 0 ? void 0 : _c.querySelector(".onepasswordSaveBtn");
            mainButton.setAttribute("title", helperText);
            this.setAttribute("visible", "");
            const infoModal = (_d = this.shadowRoot) === null || _d === void 0 ? void 0 : _d.querySelector("#onepassword-modal");
            infoModal.focus();
        }
    }
    _toggleInfoModal() {
        var _a, _b, _c, _d;
        const infoModal = (_a = this.shadowRoot) === null || _a === void 0 ? void 0 : _a.querySelector("#onepassword-modal");
        const closeButton = (_b = this.shadowRoot) === null || _b === void 0 ? void 0 : _b.querySelector("#close-icon");
        const infoButton = (_c = this.shadowRoot) === null || _c === void 0 ? void 0 : _c.querySelector("#onepassword-info-icon");
        const webComponentBody = (_d = this.shadowRoot) === null || _d === void 0 ? void 0 : _d.querySelector(".main");
        document.body.addEventListener("click", (e) => {
            if (e.target !== this) {
                this.removeAttribute("visible");
            }
        });
        webComponentBody === null || webComponentBody === void 0 ? void 0 : webComponentBody.addEventListener("click", (e) => {
            const target = e.target;
            if (this.hasAttribute("visible")
                && !(infoModal === null || infoModal === void 0 ? void 0 : infoModal.contains(target))
                && !(infoButton === null || infoButton === void 0 ? void 0 : infoButton.contains(target))) {
                this.removeAttribute("visible");
            }
        });
        closeButton === null || closeButton === void 0 ? void 0 : closeButton.addEventListener("click", (e) => {
            this.removeAttribute("visible");
        });
        closeButton === null || closeButton === void 0 ? void 0 : closeButton.addEventListener("keyup", (e) => {
            if (e.key === "Enter") {
                e.preventDefault();
                this.removeAttribute("visible");
                infoButton === null || infoButton === void 0 ? void 0 : infoButton.focus();
            }
        });
        infoButton === null || infoButton === void 0 ? void 0 : infoButton.addEventListener("click", (e) => {
            this._positionModal();
            this.setAttribute("visible", "");
            infoModal === null || infoModal === void 0 ? void 0 : infoModal.focus();
        });
        infoButton === null || infoButton === void 0 ? void 0 : infoButton.addEventListener("keyup", (e) => {
            if (e.key === "Enter") {
                e.preventDefault();
                this._positionModal();
                this.setAttribute("visible", "");
                infoModal === null || infoModal === void 0 ? void 0 : infoModal.focus();
            }
        });
    }
    _positionModal() {
        var _a, _b, _c, _d;
        const infoButtonCoords = (_b = (_a = this.shadowRoot) === null || _a === void 0 ? void 0 : _a.querySelector("#onepassword-info-icon")) === null || _b === void 0 ? void 0 : _b.getBoundingClientRect();
        if (!this.visible) {
            const modalCoords = (_d = (_c = this.shadowRoot) === null || _c === void 0 ? void 0 : _c.querySelector("#onepassword-modal")) === null || _d === void 0 ? void 0 : _d.getBoundingClientRect();
            if (modalCoords && infoButtonCoords) {
                const modalRightY = modalCoords.y + modalCoords.height;
                if (modalCoords.y < 0 || modalRightY < 0 || infoButtonCoords.y < 140) {
                    this.setAttribute("modal_position", "bottom");
                }
                else {
                    this.setAttribute("modal_position", "top");
                }
            }
        }
    }
    _focusTrap(e, direction, firstFocusableElement, lastFocusableElement) {
        var _a;
        const activeElement = (_a = this.shadowRoot) === null || _a === void 0 ? void 0 : _a.activeElement;
        if (activeElement === lastFocusableElement && direction === "next") {
            e.preventDefault();
            firstFocusableElement.focus();
        }
        else if (activeElement === firstFocusableElement && direction === "back") {
            e.preventDefault();
            lastFocusableElement.focus();
        }
    }
    _focusListener(e) {
        var _a, _b, _c;
        const modalVisible = (_b = (_a = this.shadowRoot) === null || _a === void 0 ? void 0 : _a.querySelector(".wrapper")) === null || _b === void 0 ? void 0 : _b.hasAttribute("visible");
        if (e.key !== "Tab" || !modalVisible) {
            return;
        }
        const infoModal = (_c = this.shadowRoot) === null || _c === void 0 ? void 0 : _c.querySelector("#onepassword-modal");
        const hideCta = this.hasAttribute("hide_call_to_action");
        const ctaMessage = infoModal.querySelector("#op-modal-cta");
        let firstFocusableItem = ctaMessage;
        const lastFocusableItem = infoModal.querySelector("#close-icon");
        if (hideCta) {
            if (!this.errorMessages.length) {
                ctaMessage.removeAttribute("tabindex");
                firstFocusableItem = lastFocusableItem;
            }
            else if (this.errorMessages.length) {
                firstFocusableItem = infoModal.querySelector("a")
                    ? infoModal.querySelector("a")
                    : lastFocusableItem;
            }
        }
        if (e.shiftKey) {
            this._focusTrap(e, "back", firstFocusableItem, lastFocusableItem);
        }
        else {
            this._focusTrap(e, "next", firstFocusableItem, lastFocusableItem);
        }
    }
    connectedCallback() {
        this._render();
        this._toggleInfoModal();
        this.addEventListener("keydown", this._focusListener);
    }
    disconnectedCallback() {
        this.removeEventListener("keydown", this._focusListener);
    }
    _render() {
        const container = document.createElement("div");
        container.innerHTML = `
      <style>

      span, h3, p {
        font-family: "-apple-system", "BlinkMacSystemFont", "Segoe UI", "Roboto", "Noto Sans", "Helvetica Neue", "Helvetica", "Arial", sans-serif;
      }
      h3 {
        font-weight: 600;
        font-size: 0.875rem;
      }
      .wrapper {
        flex-direction: column;
        justify-content: space-between;
        position: relative;
      }
      #onepassword-modal {
        display: flex;
        flex-direction: column;
        align-items: flex-start;
        padding: 16px;
        width: 299px;
        min-height: 94px;
        background: #FFFFFF;
        border: 1px solid rgba(0, 0, 0, 0.1);
        box-shadow: 0px 5px 10px 1px rgba(0, 0, 0, 0.1);
        border-radius: 12px;
        opacity: 0;
        visibility: hidden;
        transition: visibility 0s linear 0.1s, opacity 0.3s ease;
        position: absolute;
        text-align: left;
        z-index: 10;
      }
      [hide_call_to_action] #onepassword-modal {
        min-height: 50px;
      }
      [hide_call_to_action] #op-modal-cta {
        display: none;
      }
      [modal_position="top"] #onepassword-modal {
        bottom: 64px;
      }
      [modal_position="bottom"] #onepassword-modal {
        top: 64px;
      }
      [visible] #onepassword-modal {
        visibility: visible;
        opacity: 1;
        transition-delay: 0s;
      }
      #onepassword-modal:focus {
        box-shadow: 0px 0px 0px 3px rgb(0 119 255 / 30%), 0px 0px 0px 3px rgb(0 119 255 / 30%);
        outline: none;
      }
      [data-theme="dark"] #onepassword-modal:focus {
        box-shadow: 0px 0px 0px 3px rgb(0 119 255 / 50%), 0px 0px 0px 3px rgb(0 119 255 / 50%);
      }
      [visible] a:focus {
        border-radius: 4px;
        box-shadow: 0px 0px 0px 2px rgb(0 119 255 / 30%), 0px 0px 0px 2px rgb(0 119 255 / 30%);
        outline: none;
      }
      #header {
        width: 100%;
        height: 18px;
        left: 0px;
        top: 0px;
        line-height: 130%;
        color: #000000;
        flex: none;
        order: 0;
        flex-grow: 0;
        margin: 0px 0px;
      }
      #close-icon {
        width: 18px;
        height: 18px;
        border-radius: 24px;
        margin: -4px -4px 0 0;
        order: 1;
      }
      #close-icon:active {
        background: rgba(0, 119, 255, 0.15);
      }
      #close-icon:active path {
        fill: #0364D3;
        fill-opacity: 1;
      }
      #close-icon:focus {
        border: 2px solid rgba(0, 119, 255, 0.35);
        background: transparent;
        margin-top: -5px;
      }
      #close-icon:hover {
        background: rgba(0, 119, 255, 0.15);
      }
      #close-icon:hover path {
        fill: #0364D3;
        fill-opacity: 1;
      }
      #message {
        font-size: 0.8125rem;
        color: #000;
        order: 1;
        width: 100%;
        align-items: flex-start;
        display: flex;
        flex-grow: 1;
        flex-direction: column;
        min-height: calc(100% - 34px);
        padding: 4px 0 0 0;
      }
      #message div:first-child {
        flex-grow: 1;
        align-items: flex-start;
        margin: 0 0 4px 0;
        width: 100%;
      }
      #message a {
        color: #0472ec;
        text-decoration-color: #0472ec;
        padding: 1px;
      }
      #message a:hover {
        color: #0a2d4d;
        text-decoration-color: #0a2d4d;
      }
      #message p {
        margin: 0;
        width: 100%;
        letter-spacing: -0.015em;
      }
      .iconButton {
        cursor: pointer;
        padding: 0;
        background: transparent;
        border: none;
      }
      .iconButton:hover {
        background: rgba(0, 119, 255, 0.15);
        border-radius: 24px;
        outline: none;
      }
      [data-theme="dark"] .iconButton:hover {
        background: rgba(0, 119, 255, 0.5);
      }
      .iconButton:focus {
        border-radius: 24px;
        background: rgba(0, 119, 255, 0.3);
        outline: none;
      }
      [data-theme="dark"] .iconButton:focus {
        background: rgba(0, 119, 255, 0.5);
      }
      [data-theme="light"] .iconButton:focus circle {
        fill: #FFFFFF;
      }
      [data-theme="dark"] #onepassword-info-icon:focus path {
        fill: #FFFFFF;
      }
      #onepassword-icon {
        width: 24px;
        min-width: 24px;
      }
      [data-theme="dark"] [disabled] #onepassword-icon path {
        fill: #bbbbbb;
      }
      #onepassword-info-icon {
        display: inline-flex;
        padding: 4px;
        margin-right: 4px;
      }
      [padding="none"] .main {
        padding: 0px 0;
      }
      [padding="compact"] .main {
        padding: 16px 0;
      }
      .main {
        padding: 24px 0;
      }
      div {
        display: flex;
        align-items: center;
        justify-content: space-between;
      }
      .onepasswordSaveBtn {
        display: flex;
        flex-direction: row;
        align-items: center;
        cursor: pointer;
        margin-right: 5px;
        user-select: none;
        /* 163px equivalent */
        min-width: 10.1875rem;
        /* 165px equivalent */
        max-width: 10.3125rem;
        font-size: 0.83125rem;
      }
      .onepasswordSaveBtn:focus {
        border-radius: 8px;
        box-shadow: 0px 0px 0px 3px rgb(0 119 255 / 30%), 0px 0px 0px 3px rgb(0 119 255 / 30%);
        outline: none;
      }
      .label {
          width: 100%;
          text-align: center;
          color: #fff;
          flex: auto;
          order: 1;
          flex-grow: 0;
          padding: 3px 0px 3px 3px;
          word-break: break-word;
          align-self: center;
          line-height: 1em;
          letter-spacing: -0.1px;
          font-weight: 500;
      }
      .onepasswordSaveBtn {
        padding-bottom: 2px;
        width: auto;
        min-height: 32px;
        left: 0px;
        top: 0px;
        box-sizing: border-box;
        border-radius: 8px;
        border: 1px solid rgba(0, 0, 0, 0.3);
        background: linear-gradient(141.91deg, #0572EC 10.24%, rgba(3, 100, 211, 0.42) 81.93%), #0364D3;
      }
      .onepasswordSaveBtn:hover:enabled {
        background: #0567D5;
      }
      .onepasswordSaveBtn:active:enabled {
        background: #0450A6;
      }
      .black.onepasswordSaveBtn {
        border: 1px solid rgba(255, 255, 255, 0.1);
        background: #333333;
      }
      .black.onepasswordSaveBtn:hover:enabled {
        background: #1A1A1A;
      }
      [data-theme="dark"] .black.onepasswordSaveBtn:hover:enabled {
        background: #2d2d2d;
      }
      .black.onepasswordSaveBtn:active:enabled {
        background: #000000;
      }
      .white.onepasswordSaveBtn {
        background: #F7F7F7;
        border: 1px solid rgba(0, 0, 0, 0.3);
      }
      .white.onepasswordSaveBtn:hover:enabled {
        border: 1px solid rgba(0, 119, 255, 0.6);
        background: #F5FAFF;
      }
      [data-theme="dark"] .white.onepasswordSaveBtn:hover:enabled {
        background: #e7f3ff;
      }
      .white.onepasswordSaveBtn:active:enabled {
        border: 1px solid rgba(0, 0, 0, 0.3);
      }
      .white.onepasswordSaveBtn .label {
        color: #262626;
      }
      .white.onepasswordSaveBtn:enabled #onepassword-icon circle {
        fill: #0572EC;
      }
      .onepasswordSaveBtn:disabled {
        background: rgba(0, 0, 0, 0.06);
        border: 1px solid rgba(0, 0, 0, 0.1);
        box-sizing: border-box;
        border-radius: 8px;
        cursor: not-allowed;
      }
      [data-theme="dark"] .onepasswordSaveBtn:disabled {
        background: #aaaaaa;
      }
      .onepasswordSaveBtn:disabled .label {
        color: rgba(0, 0, 0, 0.5);
      }
      .onepasswordSaveBtn:disabled svg circle {
        fill: rgb(68 68 68 / 20%);
      }

      /* #a11y */
      .sr-only:not(:focus):not(:active) {
        clip: rect(0 0 0 0);
        clip-path: inset(100%);
        height: 1px;
        overflow: hidden;
        position: absolute;
        white-space: nowrap;
        width: 1px;
      }


      </style>

      <div class="wrapper" modal_position=${this.modal_position} data-theme=${this.color_mode} padding=${this.padding}>
        <div id="onepassword-modal" tabindex="2" role="dialog">
        <div id="header">
          <h3 id="op-modal-header">${this._localize("What's 1Password")}<span aria-hidden="false" class="sr-only">${this._localize("1Password is a secure place to keep your passwords, credit cards, and other important items and share them with your family or team.")}</span></h3>
            <button id="close-icon" class="iconButton" tabindex="4">
              <svg aria-hidden="true" width="10" height="10" viewBox="0 0 10 10" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M5 2.83056L2.46898 0.299538C2.0696 -0.0998459 1.42207 -0.0998459 1.02269 0.299538L0.299538 1.02269C-0.0998459 1.42207 -0.0998459 2.0696 0.299538 2.46898L2.83056 5L0.299538 7.53102C-0.0998456 7.9304 -0.0998459 8.57793 0.299538 8.97731L1.02269 9.70046C1.42207 10.0998 2.0696 10.0998 2.46898 9.70046L5 7.16944L7.53102 9.70046C7.9304 10.0998 8.57793 10.0998 8.97731 9.70046L9.70046 8.97731C10.0998 8.57793 10.0998 7.9304 9.70046 7.53102L7.16944 5L9.70046 2.46898C10.0998 2.0696 10.0998 1.42207 9.70046 1.02269L8.97731 0.299538C8.57793 -0.0998459 7.9304 -0.0998459 7.53102 0.299538L5 2.83056Z" fill="black" fill-opacity="0.55"/>
              </svg>
              <span aria-hidden="false" class="sr-only">${this._localize("Close this modal")}</span>
            </button>
          </div>
          <div id="message" aria-hidden="true">
            <div><p id="op-modal-body">${this._localize("1Password is a secure place to keep your passwords, credit cards, and other important items and share them with your family or team.")}</p></div>
          <div><p><a id="op-modal-cta" tabindex="3" href=${this.ctaUrls[this.lang]} target="_blank" rel="noopener noreferrer">${this._localize("Sign up for free")}</a></p></div>
          </div>
        </div>

        <div class="main">
          <button
            class="${this.class ? this.class : ""} onepasswordSaveBtn"
            tabindex="1"
            disabled
            data-onepassword-save-button
            data-onepassword-save-request
            value=${this.value}
            data-onepassword-type=${this.data_onepassword_type}
            id=${this.internalId}
            >
            <svg id="onepassword-icon" aria-hidden="true" width="24" height="25" viewBox="0 0 24 25" fill="none" xmlns="http://www.w3.org/2000/svg">
              <circle cx="12" cy="12.5" r="10"></circle>
              <path fill-rule="evenodd" clip-rule="evenodd" d="M12 21.5C6.94409 21.5 2.84547 17.4707 2.84547 12.5C2.84547 7.52943 6.94409 3.5 12 3.5C17.0559 3.5 21.1545 7.52943 21.1545 12.5C21.1545 17.4707 17.0559 21.5 12 21.5ZM19.4906 12.5009C19.4906 8.43408 16.1371 5.13724 12.0005 5.13724C7.8639 5.13724 4.51044 8.43408 4.51044 12.5009C4.51044 16.5678 7.8639 19.8645 12.0005 19.8645C16.1371 19.8645 19.4906 16.5678 19.4906 12.5009Z" fill="white"/>
              <path fill-rule="evenodd" clip-rule="evenodd" d="M17.493 12.5369C17.493 9.51835 15.0338 7.10065 12.0372 7.10065C8.96681 7.10065 6.5076 9.51835 6.5076 12.5369C6.5076 15.4831 8.96681 17.9006 12.0372 17.9006C15.0338 17.9006 17.493 15.4831 17.493 12.5369ZM13.3321 9.04622C13.3321 8.78466 13.142 8.57262 12.9076 8.57262H11.0934C10.859 8.57262 10.6689 8.78466 10.6689 9.04622V10.8016C10.6689 10.8853 10.6988 10.9656 10.7518 11.0248L11.0171 11.3208C11.1276 11.4441 11.1276 11.644 11.0171 11.7673L10.7518 12.0633C10.6988 12.1225 10.6689 12.2028 10.6689 12.2866V15.9536C10.6689 16.2151 10.859 16.4272 11.0934 16.4272H12.9076C13.142 16.4272 13.3321 16.2151 13.3321 15.9536V14.1982C13.3321 14.1145 13.3023 14.0342 13.2492 13.975L12.9839 13.679C12.8734 13.5557 12.8734 13.3558 12.9839 13.2325L13.2492 12.9365C13.3023 12.8773 13.3321 12.7969 13.3321 12.7132V9.04622Z" fill="white"/>
            </svg>

            <span class="label">${this._localize("Save in 1Password")}</span>
          </button>

          <button id="onepassword-info-icon" class="iconButton" tabindex="1">
            <svg aria-hidden="true" width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
              <circle cx="8" cy="8" r="7"></circle>
              ${this._infoIconSvg(this.color_mode)}
            </svg>
            <span aria-hidden="false" class="sr-only">${this._localize("Click to learn more about 1Password")}</span>
          </button>

        </div>

      </div>
    `;
        const shadowRoot = this.attachShadow({ mode: "open" });
        shadowRoot.appendChild(container);
    }
}
const supportsCustomElements = "customElements" in window;
if (supportsCustomElements) {
    customElements.define("onepassword-save-button", Button);
}
