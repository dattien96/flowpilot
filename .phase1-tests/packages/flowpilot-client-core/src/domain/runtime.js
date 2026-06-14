"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.SaveSupabaseConfigUseCase = exports.ValidateSupabaseConfigUseCase = exports.LoadSupabaseRuntimeStatusUseCase = void 0;
class LoadSupabaseRuntimeStatusUseCase {
    repository;
    constructor(repository) {
        this.repository = repository;
    }
    execute() {
        return this.repository.loadSupabaseRuntimeStatus();
    }
}
exports.LoadSupabaseRuntimeStatusUseCase = LoadSupabaseRuntimeStatusUseCase;
class ValidateSupabaseConfigUseCase {
    repository;
    constructor(repository) {
        this.repository = repository;
    }
    execute(input) {
        return this.repository.validateSupabaseConfig(input);
    }
}
exports.ValidateSupabaseConfigUseCase = ValidateSupabaseConfigUseCase;
class SaveSupabaseConfigUseCase {
    repository;
    constructor(repository) {
        this.repository = repository;
    }
    execute(input) {
        return this.repository.saveSupabaseConfig(input);
    }
}
exports.SaveSupabaseConfigUseCase = SaveSupabaseConfigUseCase;
