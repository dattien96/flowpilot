"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.AdminUseCases = void 0;
class AdminUseCases {
    projects;
    teams;
    workflows;
    artifacts;
    providers;
    integrations;
    directories;
    constructor(projects, teams, workflows, artifacts, providers, integrations, directories) {
        this.projects = projects;
        this.teams = teams;
        this.workflows = workflows;
        this.artifacts = artifacts;
        this.providers = providers;
        this.integrations = integrations;
        this.directories = directories;
    }
}
exports.AdminUseCases = AdminUseCases;
