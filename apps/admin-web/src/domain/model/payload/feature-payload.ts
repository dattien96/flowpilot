export interface CreateFeaturePayload {
  projectId: string;
  title: string;
  businessGoal: string;
  userProblem: string;
  expectedFlow: string;
  acceptanceCriteria: string;
  priority: "low" | "medium" | "high";
}
