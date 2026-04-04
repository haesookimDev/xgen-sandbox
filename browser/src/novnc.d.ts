declare module "@novnc/novnc/core/rfb.js" {
  interface RFBOptions {
    shared?: boolean;
    credentials?: { password?: string; username?: string; target?: string };
    wsProtocols?: string[];
  }

  export default class RFB {
    constructor(target: HTMLElement, urlOrChannel: string, options?: RFBOptions);
    scaleViewport: boolean;
    clipViewport: boolean;
    viewOnly: boolean;
    resizeSession: boolean;
    addEventListener(type: string, listener: (e: CustomEvent) => void): void;
    removeEventListener(type: string, listener: (e: CustomEvent) => void): void;
    disconnect(): void;
    sendCredentials(credentials: { password?: string }): void;
    focus(): void;
    blur(): void;
  }
}
