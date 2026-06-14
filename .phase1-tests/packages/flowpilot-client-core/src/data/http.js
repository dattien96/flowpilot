"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.fetchHttpClient = void 0;
exports.fetchHttpClient = {
    request(input, init) {
        return fetch(input, init);
    },
};
